package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/ssh"
)

// Files under the config dir (~/.config/lor). The CLI never touches ~/.ssh.
const (
	configFile     = "config.toml"
	sshConfigFile  = "ssh_config"
	knownHostsFile = "known_hosts"
	grantsDir      = "grants"
	keyFile        = "id_ed25519"
	certFile       = "id_ed25519-cert.pub"
	metaFile       = "grant.json"
	endedFile      = "ended.json"
)

// Config is ~/.config/lor/config.toml (mode 0600).
type Config struct {
	URL         string `toml:"url"`
	Fingerprint string `toml:"fingerprint"`
	Token       string `toml:"token"`
	Agent       string `toml:"agent"`
}

// State is the CLI's directory.
type State struct {
	Dir string
	Now func() time.Time
}

// DefaultDir is $LOR_CONFIG_DIR, else <user config dir>/lor
// (~/.config/lor).
func DefaultDir() (string, error) {
	if d := os.Getenv("LOR_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "lor"), nil
}

func (s *State) path(parts ...string) string {
	return filepath.Join(append([]string{s.Dir}, parts...)...)
}

// LoadConfig reads config.toml.
func (s *State) LoadConfig() (*Config, error) {
	data, err := os.ReadFile(s.path(configFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("not enrolled; run the `lor enroll …` command from Lor's Agents page")
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if _, err := toml.Decode(string(data), &c); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(configFile), err)
	}
	if c.URL == "" || c.Fingerprint == "" || c.Token == "" {
		return nil, fmt.Errorf("%s is incomplete; enroll again", s.path(configFile))
	}
	return &c, nil
}

// SaveConfig writes config.toml with mode 0600.
func (s *State) SaveConfig(c *Config) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	os.Chmod(s.Dir, 0o700)
	var b strings.Builder
	b.WriteString("# Written by `lor enroll`. Keep private: the token identifies this agent.\n")
	if err := toml.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	return writeFileAtomic(s.path(configFile), []byte(b.String()), 0o600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LocalGrant is grants/<id>/ on disk.
type LocalGrant struct {
	ID          int64     `json:"id"`
	Host        string    `json:"host"`
	Level       string    `json:"level"`
	User        string    `json:"user,omitempty"`
	Hostname    string    `json:"hostname,omitempty"`
	Port        int       `json:"port,omitempty"`
	Alias       string    `json:"host_key_alias,omitempty"`
	ValidBefore time.Time `json:"valid_before,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *State) grantDir(id int64) string { return s.path(grantsDir, strconv.FormatInt(id, 10)) }

// KeyPath is the private key of a grant.
func (s *State) KeyPath(id int64) string { return filepath.Join(s.grantDir(id), keyFile) }

// CertPath is the certificate of a grant.
func (s *State) CertPath(id int64) string { return filepath.Join(s.grantDir(id), certFile) }

// NewKey generates an ed25519 key in a temporary grant directory and
// returns the directory and the public key in authorized_keys format.
func (s *State) NewKey() (string, string, error) {
	root := s.path(grantsDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", err
	}
	dir, err := os.MkdirTemp(root, ".new-")
	if err != nil {
		return "", "", err
	}
	os.Chmod(dir, 0o700)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, "lor")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(dir, keyFile), pem.EncodeToMemory(block), 0o600); err != nil {
		return "", "", err
	}
	pk, err := ssh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk)))
	if err := os.WriteFile(filepath.Join(dir, keyFile+".pub"), []byte(authorized+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return dir, authorized, nil
}

// AdoptKey moves a temporary key directory to grants/<id>.
func (s *State) AdoptKey(tmpDir string, g LocalGrant) error {
	dst := s.grantDir(g.ID)
	if _, err := os.Stat(dst); err == nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("grants/%d already exists", g.ID)
	}
	if err := os.Rename(tmpDir, dst); err != nil {
		return err
	}
	g.CreatedAt = s.Now()
	return s.saveMeta(g)
}

func (s *State) saveMeta(g LocalGrant) error {
	b, _ := json.MarshalIndent(g, "", "  ")
	return writeFileAtomic(filepath.Join(s.grantDir(g.ID), metaFile), b, 0o600)
}

// HasKey reports whether the grant's private key is on this machine.
func (s *State) HasKey(id int64) bool {
	_, err := os.Stat(s.KeyPath(id))
	return err == nil
}

// PublicKey returns the grant's public key in authorized_keys format.
func (s *State) PublicKey(id int64) (string, error) {
	b, err := os.ReadFile(s.KeyPath(id) + ".pub")
	return strings.TrimSpace(string(b)), err
}

// SaveCert stores a grant's current certificate and connection details.
func (s *State) SaveCert(g LocalGrant, cert string) error {
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(cert))
	if err != nil {
		return fmt.Errorf("invalid certificate from Lor: %w", err)
	}
	c, ok := pk.(*ssh.Certificate)
	if !ok {
		return errors.New("Lor returned a key, not a certificate")
	}
	g.ValidBefore = time.Unix(int64(c.ValidBefore), 0)
	if err := writeFileAtomic(s.CertPath(g.ID), []byte(strings.TrimSpace(cert)+"\n"), 0o600); err != nil {
		return err
	}
	if old, err := s.loadMeta(g.ID); err == nil {
		g.CreatedAt = old.CreatedAt
	} else {
		g.CreatedAt = s.Now()
	}
	return s.saveMeta(g)
}

func (s *State) loadMeta(id int64) (LocalGrant, error) {
	var g LocalGrant
	b, err := os.ReadFile(filepath.Join(s.grantDir(id), metaFile))
	if err != nil {
		return g, err
	}
	err = json.Unmarshal(b, &g)
	return g, err
}

// CertValidBefore returns the expiry of the grant's local certificate.
func (s *State) CertValidBefore(id int64) (time.Time, bool) {
	b, err := os.ReadFile(s.CertPath(id))
	if err != nil {
		return time.Time{}, false
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey(b)
	if err != nil {
		return time.Time{}, false
	}
	c, ok := pk.(*ssh.Certificate)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(c.ValidBefore), 0), true
}

// RemoveGrant deletes grants/<id>.
func (s *State) RemoveGrant(id int64) error { return os.RemoveAll(s.grantDir(id)) }

// LocalGrants lists grant directories with metadata.
func (s *State) LocalGrants() []LocalGrant {
	entries, err := os.ReadDir(s.path(grantsDir))
	if err != nil {
		return nil
	}
	var out []LocalGrant
	for _, e := range entries {
		id, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil || !e.IsDir() {
			continue
		}
		g, err := s.loadMeta(id)
		if err != nil {
			continue
		}
		out = append(out, g)
	}
	return out
}

// Cleanup deletes what is certainly dead (run on every invocation):
// abandoned temporary keys and keys that never got a certificate within a
// day. It returns the grants whose local certificate has expired; they may
// have been extended on the server, so the caller asks Lor about each one
// before deleting it.
func (s *State) Cleanup() []int64 {
	root := s.path(grantsDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	now := s.Now()
	var expired []int64
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		if strings.HasPrefix(e.Name(), ".new-") {
			if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > time.Hour {
				os.RemoveAll(p)
			}
			continue
		}
		id, err := strconv.ParseInt(e.Name(), 10, 64)
		if err != nil {
			continue
		}
		if vb, ok := s.CertValidBefore(id); ok {
			if !now.Before(vb) {
				expired = append(expired, id)
			}
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
			os.RemoveAll(p)
		}
	}
	return expired
}

// WriteSSHConfig regenerates ssh_config (one "Host lor-<host>" entry per host
// with a valid local certificate) and known_hosts. preferred, if non-zero,
// wins for its host.
func (s *State) WriteSSHConfig(preferred int64, knownHosts map[string]string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	now := s.Now()
	best := map[string]LocalGrant{}
	for _, g := range s.LocalGrants() {
		vb, ok := s.CertValidBefore(g.ID)
		if !ok || !now.Before(vb) || g.User == "" || g.Hostname == "" {
			continue
		}
		g.ValidBefore = vb
		cur, have := best[g.Host]
		switch {
		case !have, g.ID == preferred:
			best[g.Host] = g
		case cur.ID == preferred:
		case g.Level == "sudo" && cur.Level != "sudo",
			g.Level == cur.Level && g.ValidBefore.After(cur.ValidBefore):
			best[g.Host] = g
		}
	}
	hostsSorted := make([]string, 0, len(best))
	for h := range best {
		hostsSorted = append(hostsSorted, h)
	}
	sort.Strings(hostsSorted)
	var b strings.Builder
	b.WriteString("# Generated by lor; rewritten on every run. Use: ssh -F \"$(lor config-path)\" lor-<host>\n")
	for _, h := range hostsSorted {
		g := best[h]
		port := g.Port
		if port == 0 {
			port = 22
		}
		fmt.Fprintf(&b, "\nHost lor-%s\n", h)
		fmt.Fprintf(&b, "  HostName %s\n", g.Hostname)
		if port != 22 {
			fmt.Fprintf(&b, "  Port %d\n", port)
		}
		fmt.Fprintf(&b, "  User %s\n", g.User)
		fmt.Fprintf(&b, "  IdentityFile %s\n", quoteSSH(s.KeyPath(g.ID)))
		fmt.Fprintf(&b, "  CertificateFile %s\n", quoteSSH(s.CertPath(g.ID)))
		b.WriteString("  IdentitiesOnly yes\n")
		fmt.Fprintf(&b, "  HostKeyAlias %s\n", g.Alias)
		fmt.Fprintf(&b, "  UserKnownHostsFile %s\n", quoteSSH(s.path(knownHostsFile)))
		b.WriteString("  GlobalKnownHostsFile none\n")
		b.WriteString("  StrictHostKeyChecking yes\n")
		b.WriteString("  UpdateHostKeys no\n")
		b.WriteString("  BatchMode yes\n")
		b.WriteString("  IdentityAgent none\n")
		b.WriteString("  ForwardAgent no\n")
	}
	if err := writeFileAtomic(s.path(sshConfigFile), []byte(b.String()), 0o600); err != nil {
		return err
	}
	return s.updateKnownHosts(knownHosts)
}

func quoteSSH(p string) string {
	if strings.ContainsAny(p, " \t\"") {
		return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return p
}

// updateKnownHosts replaces the lines for the given aliases.
func (s *State) updateKnownHosts(lines map[string]string) error {
	if len(lines) == 0 {
		if _, err := os.Stat(s.path(knownHostsFile)); err == nil {
			return nil
		}
	}
	existing, _ := os.ReadFile(s.path(knownHostsFile))
	var out []string
	for _, l := range strings.Split(string(existing), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		alias := strings.Fields(l)[0]
		if _, replace := lines[alias]; replace {
			continue
		}
		out = append(out, l)
	}
	aliases := make([]string, 0, len(lines))
	for a := range lines {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	for _, a := range aliases {
		out = append(out, strings.TrimSpace(lines[a]))
	}
	return writeFileAtomic(s.path(knownHostsFile), []byte(strings.Join(out, "\n")+"\n"), 0o600)
}

// SSHConfigPath is the generated ssh_config.
func (s *State) SSHConfigPath() string { return s.path(sshConfigFile) }

// EndedGrant remembers a grant whose key was deleted because it ended, so a
// later `lor ssh` can say why access is gone ("expired", "revoked") instead
// of a generic "no access".
type EndedGrant struct {
	ID     int64     `json:"id"`
	Host   string    `json:"host"`
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}

const endedKeep = 24 * time.Hour

func (s *State) loadEnded() []EndedGrant {
	var list []EndedGrant
	b, err := os.ReadFile(s.path(endedFile))
	if err == nil {
		json.Unmarshal(b, &list)
	}
	return list
}

// RecordEnded remembers that a grant ended (kept for a day, at most 50).
func (s *State) RecordEnded(e EndedGrant) {
	if e.Host == "" {
		return
	}
	now := s.Now()
	e.At = now
	list := []EndedGrant{e}
	for _, x := range s.loadEnded() {
		if x.ID != e.ID && now.Sub(x.At) < endedKeep && len(list) < 50 {
			list = append(list, x)
		}
	}
	b, _ := json.Marshal(list)
	os.MkdirAll(s.Dir, 0o700)
	writeFileAtomic(s.path(endedFile), b, 0o600)
}

// RecentEnded returns the most recent ended grant for host within a day.
func (s *State) RecentEnded(host string) *EndedGrant {
	now := s.Now()
	for _, x := range s.loadEnded() {
		if x.Host == host && now.Sub(x.At) < endedKeep {
			x := x
			return &x
		}
	}
	return nil
}

// LocalHost returns the host recorded for a local grant, if any.
func (s *State) LocalHost(id int64) string {
	g, err := s.loadMeta(id)
	if err != nil {
		return ""
	}
	return g.Host
}
