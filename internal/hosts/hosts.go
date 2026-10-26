// Package hosts manages SSH targets: validation, host-key scanning and the
// signed bootstrap-script links (plan section 4).
package hosts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/apierr"
	"github.com/joshualent/lor/internal/ca"
	"github.com/joshualent/lor/internal/store"
	"github.com/joshualent/lor/scripts"
)

// BootstrapLinkTTL is how long a bootstrap link works.
const BootstrapLinkTTL = 15 * time.Minute

// ScanTimeout bounds a host-key scan.
const ScanTimeout = 10 * time.Second

const maxNotes = 2000

var bootstrapTmpl = template.Must(template.New("bootstrap").Parse(scripts.BootstrapTemplate))

// ValidateAddress accepts a LAN IP, optionally with a port ("192.168.1.20" or
// "192.168.1.20:2222"). DNS names are refused: hosts are always addressed by
// IP so the path (and source IP) can't change behind Lor's back.
func ValidateAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if ip, err := netip.ParseAddr(s); err == nil && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast() {
		return ip.Unmap().String(), nil
	}
	ap, err := netip.ParseAddrPort(s)
	if err != nil || ap.Addr().Zone() != "" || ap.Addr().IsUnspecified() || ap.Port() == 0 {
		return "", apierr.BadRequest("invalid_address", "address must be a LAN IP (optionally IP:port), not a name: %q", s)
	}
	if ap.Port() == 22 {
		return ap.Addr().Unmap().String(), nil
	}
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()).String(), nil
}

// SplitAddress returns the IP and port (22 if none) of a stored address.
func SplitAddress(addr string) (string, int) {
	if ap, err := netip.ParseAddrPort(addr); err == nil {
		return ap.Addr().String(), int(ap.Port())
	}
	return addr, 22
}

// DialAddress returns host:port for dialing.
func DialAddress(addr string) string {
	ip, port := SplitAddress(addr)
	return net.JoinHostPort(ip, strconv.Itoa(port))
}

// Scanner fetches a host's ed25519 host key.
type Scanner func(ctx context.Context, addr string) (ssh.PublicKey, error)

var errGotKey = errors.New("host key captured")

// Scan connects to addr (IP[:port]) and returns its ed25519 host key. It never
// authenticates.
func Scan(ctx context.Context, addr string) (ssh.PublicKey, error) {
	ctx, cancel := context.WithTimeout(ctx, ScanTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", DialAddress(addr))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	var key ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User:              "lor-host-key-scan",
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			key = k
			return errGotKey
		},
		Timeout: ScanTimeout,
	}
	_, _, _, err = ssh.NewClientConn(conn, DialAddress(addr), cfg)
	if key != nil {
		if key.Type() != ssh.KeyAlgoED25519 {
			return nil, fmt.Errorf("host offered a %s key; Lor needs ed25519", key.Type())
		}
		return key, nil
	}
	if err == nil {
		err = errors.New("no host key received")
	}
	return nil, fmt.Errorf("ssh handshake: %w", err)
}

// MarshalKey renders a public key in authorized_keys format without comment.
func MarshalKey(k ssh.PublicKey) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(k)))
}

// Fingerprint returns the SHA256 fingerprint of a stored host key, or "".
func Fingerprint(hostKey string) string {
	if hostKey == "" {
		return ""
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(k)
}

// Service manages hosts.
type Service struct {
	db        *store.DB
	ca        *ca.CA
	publicURL string
	linkKey   []byte
	scan      Scanner
	now       func() time.Time
}

// New returns a host service. scan may be nil (Scan).
func New(ctx context.Context, db *store.DB, authority *ca.CA, publicURL string, scan Scanner, now func() time.Time) (*Service, error) {
	if scan == nil {
		scan = Scan
	}
	if now == nil {
		now = time.Now
	}
	key, err := db.GetOrCreateMeta(ctx, "bootstrap_link_key", func() []byte {
		b := make([]byte, 32)
		rand.Read(b)
		return b
	})
	if err != nil {
		return nil, err
	}
	return &Service{db: db, ca: authority, publicURL: publicURL, linkKey: key, scan: scan, now: now}, nil
}

// Input is a new or edited host.
type Input struct {
	Name       string
	Address    string
	MaxLevel   string
	Hypervisor bool
	LorInfra   bool
	Notes      string
}

func validNotes(n string) (string, error) {
	n = strings.TrimSpace(n)
	if utf8.RuneCountInString(n) > maxNotes || !utf8.ValidString(n) {
		return "", apierr.BadRequest("invalid_notes", "notes must be at most %d characters", maxNotes)
	}
	return n, nil
}

func validLevel(l string) error {
	if l != "user" && l != "sudo" {
		return apierr.BadRequest("invalid_level", "max level must be user or sudo")
	}
	return nil
}

func audit(ctx context.Context, tx *store.Tx, now time.Time, actor, action string, hostID int64, details map[string]any) error {
	return tx.InsertAudit(ctx, &store.AuditEvent{TS: now.Unix(), Actor: actor, Action: action, HostID: hostID, Details: details})
}

// Create adds a host and scans its key. A scan failure doesn't fail the
// call: the host is created without a key and scanErr says why.
func (s *Service) Create(ctx context.Context, actor string, in Input) (h *store.Host, scanErr string, err error) {
	if !ca.HostNameRe.MatchString(in.Name) {
		return nil, "", apierr.BadRequest("invalid_name", "host name must be lowercase letters, digits and dashes (max 63); it becomes the certificate principal")
	}
	addr, err := ValidateAddress(in.Address)
	if err != nil {
		return nil, "", err
	}
	if err := validLevel(in.MaxLevel); err != nil {
		return nil, "", err
	}
	notes, err := validNotes(in.Notes)
	if err != nil {
		return nil, "", err
	}
	h = &store.Host{Name: in.Name, Address: addr, MaxLevel: in.MaxLevel, Hypervisor: in.Hypervisor, LorInfra: in.LorInfra,
		Notes: notes, CreatedAt: s.now().Unix()}
	err = s.db.WithTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.GetHostByName(ctx, in.Name); err == nil {
			return apierr.Conflict("host_exists", "a host named %s already exists", in.Name)
		}
		if err := tx.CreateHost(ctx, h); err != nil {
			return err
		}
		return audit(ctx, tx, s.now(), actor, "host.created", h.ID, map[string]any{
			"name": h.Name, "address": h.Address, "max_level": h.MaxLevel, "hypervisor": h.Hypervisor, "lor_infra": h.LorInfra,
		})
	})
	if err != nil {
		return nil, "", err
	}
	res, err := s.Rescan(ctx, actor, h.Name)
	if err != nil {
		var ae *apierr.Error
		if errors.As(err, &ae) {
			return h, ae.Message, nil
		}
		return h, err.Error(), nil
	}
	return res.Host, "", nil
}

// Update is a partial host edit. The host key is never cleared here: a new
// address gets rescanned, and a different key needs step-up to accept.
type Update struct {
	Name       *string
	Address    *string
	MaxLevel   *string
	Hypervisor *bool
	LorInfra   *bool
	Notes      *string
	Disabled   *bool
}

// Edit applies an Update.
func (s *Service) Edit(ctx context.Context, actor, name string, u Update) (*store.Host, error) {
	var h *store.Host
	err := s.db.WithTx(ctx, func(tx *store.Tx) error {
		var err error
		h, err = tx.GetHostByName(ctx, name)
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("host_not_found", "host not found")
		}
		if err != nil {
			return err
		}
		changes := map[string]any{}
		if u.Name != nil && *u.Name != h.Name {
			newName := strings.TrimSpace(*u.Name)
			if !ca.HostNameRe.MatchString(newName) {
				return apierr.BadRequest("invalid_name", "host name must be lowercase letters, digits and dashes (max 63); it becomes the certificate principal")
			}
			if _, err := tx.GetHostByName(ctx, newName); err == nil {
				return apierr.Conflict("host_exists", "a host named %s already exists", newName)
			}
			// The name is the certificate principal: renaming with open grants
			// would leave certificates and the host's principals out of step.
			open, err := tx.ListGrants(ctx, store.GrantFilter{HostID: h.ID, Statuses: []string{store.StatusPending, store.StatusOffered, store.StatusActive}, Limit: 1})
			if err != nil {
				return err
			}
			if len(open) > 0 {
				return apierr.Conflict("host_in_use", "end or wait out the open grants on %s before renaming it", h.Name)
			}
			changes["name"] = []string{h.Name, newName}
			h.Name = newName
		}
		if u.Address != nil {
			addr, err := ValidateAddress(*u.Address)
			if err != nil {
				return err
			}
			if addr != h.Address {
				changes["address"] = []string{h.Address, addr}
				h.Address = addr
			}
		}
		if u.MaxLevel != nil {
			if err := validLevel(*u.MaxLevel); err != nil {
				return err
			}
			if *u.MaxLevel != h.MaxLevel {
				changes["max_level"] = []string{h.MaxLevel, *u.MaxLevel}
				h.MaxLevel = *u.MaxLevel
			}
		}
		if u.Hypervisor != nil && *u.Hypervisor != h.Hypervisor {
			changes["hypervisor"] = *u.Hypervisor
			h.Hypervisor = *u.Hypervisor
		}
		if u.LorInfra != nil && *u.LorInfra != h.LorInfra {
			changes["lor_infra"] = *u.LorInfra
			h.LorInfra = *u.LorInfra
		}
		if u.Disabled != nil && *u.Disabled != h.Disabled {
			changes["disabled"] = *u.Disabled
			h.Disabled = *u.Disabled
		}
		if u.Notes != nil {
			n, err := validNotes(*u.Notes)
			if err != nil {
				return err
			}
			if n != h.Notes {
				changes["notes"] = true
				h.Notes = n
			}
		}
		if len(changes) == 0 {
			return nil
		}
		if err := tx.UpdateHost(ctx, h); err != nil {
			return err
		}
		return audit(ctx, tx, s.now(), actor, "host.updated", h.ID, changes)
	})
	return h, err
}

// ScanResult is the outcome of Rescan.
type ScanResult struct {
	Host        *store.Host
	Key         string
	Fingerprint string
	Changed     bool   // differs from the stored key; needs AcceptKey (step-up)
	PreviousFP  string // stored key's fingerprint when Changed
}

// Rescan scans the host. The first key is stored directly (and audited); a
// different key is only reported, never stored, until accepted with step-up.
func (s *Service) Rescan(ctx context.Context, actor, name string) (*ScanResult, error) {
	h, err := s.db.GetHostByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, apierr.NotFound("host_not_found", "host not found")
	}
	if err != nil {
		return nil, err
	}
	k, err := s.scan(ctx, h.Address)
	if err != nil {
		return nil, apierr.New(502, "scan_failed", "couldn't read the host key from %s: %v", h.Address, err)
	}
	key := MarshalKey(k)
	res := &ScanResult{Host: h, Key: key, Fingerprint: ssh.FingerprintSHA256(k)}
	if h.HostKey == key {
		return res, nil
	}
	if h.HostKey != "" {
		res.Changed = true
		res.PreviousFP = Fingerprint(h.HostKey)
		err := s.db.WithTx(ctx, func(tx *store.Tx) error {
			return audit(ctx, tx, s.now(), actor, "host.key_changed_detected", h.ID, map[string]any{
				"previous": res.PreviousFP, "scanned": res.Fingerprint,
			})
		})
		return res, err
	}
	err = s.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetFirstHostKey(ctx, h.ID, h.Address, key, s.now().Unix()); err != nil {
			return err
		}
		return audit(ctx, tx, s.now(), actor, "host.key_scanned", h.ID, map[string]any{"fingerprint": res.Fingerprint})
	})
	if errors.Is(err, store.ErrConflict) {
		// Another scan stored a key (or the address changed) meanwhile:
		// compare against that instead of overwriting it.
		return s.Rescan(ctx, actor, name)
	}
	if err != nil {
		return nil, err
	}
	h.HostKey, h.HostKeySeenAt = key, s.now().Unix()
	return res, nil
}

// CheckAcceptKey validates accepting a changed key (step-up begin).
func (s *Service) CheckAcceptKey(ctx context.Context, hostID int64, key string) (*store.Host, string, error) {
	h, err := s.db.GetHost(ctx, hostID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, "", apierr.NotFound("host_not_found", "host not found")
	}
	if err != nil {
		return nil, "", err
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil || k.Type() != ssh.KeyAlgoED25519 {
		return nil, "", apierr.BadRequest("invalid_host_key", "host key must be an ed25519 public key")
	}
	if MarshalKey(k) == h.HostKey {
		return nil, "", apierr.Conflict("key_unchanged", "that key is already stored")
	}
	return h, ssh.FingerprintSHA256(k), nil
}

// AcceptKey stores a changed host key (after step-up).
func (s *Service) AcceptKey(ctx context.Context, actor string, hostID int64, key string) (*store.Host, error) {
	h, fp, err := s.CheckAcceptKey(ctx, hostID, key)
	if err != nil {
		return nil, err
	}
	k, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(key))
	prev := Fingerprint(h.HostKey)
	err = s.db.WithTx(ctx, func(tx *store.Tx) error {
		if err := tx.SetHostKey(ctx, h.ID, MarshalKey(k), s.now().Unix()); err != nil {
			return err
		}
		return audit(ctx, tx, s.now(), actor, "host.key_changed_accepted", h.ID, map[string]any{"previous": prev, "accepted": fp})
	})
	if err != nil {
		return nil, err
	}
	return s.db.GetHost(ctx, hostID)
}

// ---------------------------------------------------------------- bootstrap links

func (s *Service) mac(name string, exp int64) []byte {
	m := hmac.New(sha256.New, s.linkKey)
	fmt.Fprintf(m, "lor-bootstrap|%s|%d", name, exp)
	return m.Sum(nil)[:16]
}

// BootstrapToken returns a signed token for name, valid for 15 minutes.
func (s *Service) BootstrapToken(name string) (string, time.Time) {
	exp := s.now().Add(BootstrapLinkTTL)
	b := make([]byte, 8, 24)
	binary.BigEndian.PutUint64(b, uint64(exp.Unix()))
	b = append(b, s.mac(name, exp.Unix())...)
	return base64.RawURLEncoding.EncodeToString(b), exp
}

// VerifyBootstrapToken checks a token for name.
func (s *Service) VerifyBootstrapToken(name, token string) bool {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 24 {
		return false
	}
	exp := int64(binary.BigEndian.Uint64(b[:8]))
	now := s.now()
	if now.Unix() >= exp || exp > now.Add(BootstrapLinkTTL).Unix()+1 {
		return false
	}
	return hmac.Equal(b[8:], s.mac(name, exp))
}

// Bootstrap holds the one-liners shown on the host page.
type Bootstrap struct {
	Install       string
	InstallNoSudo string
	Uninstall     string
	ExpiresAt     time.Time
}

// BootstrapLinks returns fresh one-liners for a host.
func (s *Service) BootstrapLinks(h *store.Host) Bootstrap {
	tok, exp := s.BootstrapToken(h.Name)
	link := fmt.Sprintf("%s/bootstrap/%s.sh?t=%s", s.publicURL, url.PathEscape(h.Name), tok)
	base := fmt.Sprintf("curl -fsSL '%s' | ssh %s sudo sh", link, h.Name)
	install := base
	if h.MaxLevel != "sudo" {
		install = base + " -s -- --no-sudo"
	}
	return Bootstrap{
		Install:       install,
		InstallNoSudo: base + " -s -- --no-sudo",
		Uninstall:     base + " -s -- --uninstall",
		ExpiresAt:     exp,
	}
}

// RenderBootstrap renders the bootstrap script for a host.
func (s *Service) RenderBootstrap(ctx context.Context, h *store.Host) ([]byte, error) {
	if !ca.HostNameRe.MatchString(h.Name) {
		return nil, errors.New("invalid host name")
	}
	addrs, err := s.agentAddrs(ctx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = bootstrapTmpl.Execute(&buf, struct{ Host, CAPub, AgentAddrs string }{h.Name, s.ca.AuthorizedKey(), addrs})
	return buf.Bytes(), err
}

// agentAddrs lists every registered agent LAN IP (space-separated) for the
// bootstrap script's validation. Addresses were validated as zone-free IPs
// on input; they're re-checked here because they go into a shell script.
func (s *Service) agentAddrs(ctx context.Context) (string, error) {
	agents, err := s.db.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range agents {
		for _, ip := range a.LANAddrs {
			if p, err := netip.ParseAddr(ip); err == nil && p.Zone() == "" && !seen[p.String()] {
				seen[p.String()] = true
				out = append(out, p.String())
			}
		}
	}
	return strings.Join(out, " "), nil
}
