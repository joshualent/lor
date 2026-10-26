//go:build integration

// Package integration runs Lor against real OpenSSH servers in Docker
// containers (plan section 13). One run covers one distro, chosen with
// DISTRO (default debian:12). Requires Docker and `make build`.
//
// Layout on a private Docker network (172.30.99.0/24):
//
//	172.30.99.1   the test runner: lor-server (agent listener on this IP),
//	              the admin API client (software passkey) and a Go SSH
//	              client with hand-minted certificates
//	172.30.99.10  agent container: the real `lor` CLI + OpenSSH client
//	172.30.99.20  target "nas"     (bootstrapped)
//	172.30.99.21  target "pihole"  (bootstrapped)
//	172.30.99.22  target "rb"      (rollback test)
package integration

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/joshualent/lor/internal/auth/authtest"
)

const (
	subnet    = "172.30.99.0/24"
	gateway   = "172.30.99.1"
	agentIP   = "172.30.99.10"
	nasIP     = "172.30.99.20"
	piholeIP  = "172.30.99.21"
	rbIP      = "172.30.99.22"
	publicURL = "https://lor.test"
	adminAddr = "127.0.0.1:18444"
	agentAddr = gateway + ":18443"
)

type env struct {
	t        *testing.T
	distro   string
	image    string
	network  string
	root     string
	dataDir  string
	server   *exec.Cmd
	admin    *http.Client
	passkey  *authtest.Passkey
	caSigner ssh.Signer
	prefix   string
}

func run(t *testing.T, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func must(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := run(t, name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func newEnv(t *testing.T) *env {
	distro := os.Getenv("DISTRO")
	if distro == "" {
		distro = "debian:12"
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root, _ := filepath.Abs("../..")
	for _, b := range []string{"lor", "lor-server"} {
		if _, err := os.Stat(filepath.Join(root, "bin", b)); err != nil {
			t.Fatalf("bin/%s missing: run `make build` first", b)
		}
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e6)
	e := &env{
		t: t, distro: distro, root: root,
		image:   "lor-it-target:" + strings.NewReplacer(":", "-", "/", "-").Replace(distro),
		network: "lor-it-" + suffix,
		prefix:  "lor-it-" + suffix + "-",
	}
	t.Logf("distro %s", distro)
	must(t, "docker", "build", "-q", "--build-arg", "BASE="+distro, "-t", e.image, "-f",
		filepath.Join(root, "test/integration/target.Dockerfile"), filepath.Join(root, "test/integration"))
	must(t, "docker", "network", "create", "--subnet", subnet, "--gateway", gateway, e.network)
	t.Cleanup(e.cleanup)
	for name, ip := range map[string]string{"agent": agentIP, "nas": nasIP, "pihole": piholeIP, "rb": rbIP} {
		args := []string{"run", "-d", "--init", "--name", e.prefix + name, "--hostname", name, "--network", e.network, "--ip", ip}
		if name == "agent" {
			args = append(args, "-v", filepath.Join(root, "bin")+":/opt/lor:ro")
		}
		must(t, "docker", append(args, e.image)...)
	}
	for _, ip := range []string{nasIP, piholeIP, rbIP, agentIP} {
		waitSSH(t, ip)
	}
	e.startServer()
	return e
}

func (e *env) cleanup() {
	if e.server != nil && e.server.Process != nil {
		e.server.Process.Kill()
		e.server.Wait()
	}
	if e.t.Failed() {
		for _, n := range []string{"nas", "pihole", "rb", "agent"} {
			out, _ := run(e.t, "docker", "logs", "--tail", "50", e.prefix+n)
			e.t.Logf("--- docker logs %s:\n%s", n, out)
		}
	}
	for _, n := range []string{"agent", "nas", "pihole", "rb"} {
		run(e.t, "docker", "rm", "-f", e.prefix+n)
	}
	run(e.t, "docker", "network", "rm", e.network)
}

func waitSSH(t *testing.T, ip string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", ip+":22", time.Second)
		if err == nil {
			c.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 4)
			n, _ := c.Read(buf)
			c.Close()
			if n == 4 && string(buf) == "SSH-" {
				return
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("sshd on %s did not come up", ip)
}

func (e *env) startServer() {
	t := e.t
	e.dataDir = t.TempDir()
	cfg := filepath.Join(e.dataDir, "lor-server.toml")
	os.WriteFile(cfg, []byte(fmt.Sprintf(`data_dir = %q
public_url = %q
admin_listen = %q
trusted_proxy = "127.0.0.1"
agent_listen = %q
`, filepath.Join(e.dataDir, "data"), publicURL, adminAddr, agentAddr)), 0o600)
	e.server = exec.Command(filepath.Join(e.root, "bin/lor-server"), "serve", "--config", cfg)
	logf, _ := os.Create(filepath.Join(e.dataDir, "server.log"))
	e.server.Stdout, e.server.Stderr = logf, logf
	if err := e.server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			b, _ := os.ReadFile(filepath.Join(e.dataDir, "server.log"))
			t.Logf("--- lor-server log:\n%s", b)
		}
	})
	jar, _ := cookiejar.New(nil)
	e.admin = &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test client; the CLI pins
	}}
	deadline := time.Now().Add(20 * time.Second)
	for {
		resp, err := e.admin.Get("https://" + adminAddr + "/api/admin/v1/session")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("lor-server didn't start: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	keyPEM, err := os.ReadFile(filepath.Join(e.dataDir, "data/ca/lor_ca"))
	if err != nil {
		t.Fatal(err)
	}
	if e.caSigner, err = ssh.ParsePrivateKey(keyPEM); err != nil {
		t.Fatal(err)
	}

	// First passkey: setup code from the CLI, then the WebAuthn ceremony.
	out := must(t, filepath.Join(e.root, "bin/lor-server"), "admin", "setup-code", "--config", cfg)
	code := regexp.MustCompile(`Setup code: (\S+)`).FindStringSubmatch(out)[1]
	e.passkey = authtest.New("lor.test", publicURL, false)
	var begin struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	e.post("/setup/begin", map[string]any{"code": code, "passkey_name": "integration"}, &begin)
	cred, err := e.passkey.Register(begin.Options)
	if err != nil {
		t.Fatal(err)
	}
	e.post("/setup/finish", map[string]any{"ceremony_id": begin.CeremonyID, "credential": json.RawMessage(cred)}, nil)
}

// do calls the admin API (through "Caddy" = 127.0.0.1) with the CSRF headers.
func (e *env) do(method, path string, body any, out any) (int, []byte) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "https://"+adminAddr+"/api/admin/v1"+path, rd)
	if method != http.MethodGet {
		req.Header.Set("Origin", publicURL)
		req.Header.Set("X-Lor-Request", "1")
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.admin.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode == 200 {
		if err := json.Unmarshal(b, out); err != nil {
			e.t.Fatalf("%s %s: %v %s", method, path, err, b)
		}
	}
	return resp.StatusCode, b
}

func (e *env) post(path string, body, out any) {
	e.t.Helper()
	if code, b := e.do("POST", path, body, out); code != 200 {
		e.t.Fatalf("POST %s: %d %s", path, code, b)
	}
}

func (e *env) get(path string, out any) {
	e.t.Helper()
	if code, b := e.do("GET", path, nil, out); code != 200 {
		e.t.Fatalf("GET %s: %d %s", path, code, b)
	}
}

// stepUp runs a step-up action with the software passkey.
func (e *env) stepUp(action map[string]any) map[string]any {
	e.t.Helper()
	var begin struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	e.post("/stepup/begin", map[string]any{"action": action}, &begin)
	cred, err := e.passkey.Assert(begin.Options)
	if err != nil {
		e.t.Fatal(err)
	}
	var out map[string]any
	e.post("/stepup/finish", map[string]any{"ceremony_id": begin.CeremonyID, "credential": json.RawMessage(cred)}, &out)
	return out
}

// exec runs a command in a container, returning combined output and exit code.
func (e *env) exec(container string, stdin []byte, args ...string) (string, string, int) {
	e.t.Helper()
	full := append([]string{"exec", "-i", e.prefix + container}, args...)
	cmd := exec.Command("docker", full...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

// lor runs the real CLI in the agent container.
func (e *env) lor(args ...string) (string, string, int) {
	e.t.Helper()
	return e.exec("agent", nil, append([]string{"env", "HOME=/root", "/opt/lor/lor"}, args...)...)
}

// bootstrap fetches the host's bootstrap script through its signed link (as
// curl would) and runs it as root on the target ("| ssh host sudo sh").
func (e *env) bootstrap(container, host string, flags ...string) (string, int) {
	e.t.Helper()
	var d struct {
		Bootstrap struct{ Install string } `json:"bootstrap"`
	}
	e.get("/hosts/"+host, &d)
	link := regexp.MustCompile(`'(https://[^']+)'`).FindStringSubmatch(d.Bootstrap.Install)[1]
	path := strings.TrimPrefix(link, publicURL)
	resp, err := e.admin.Get("https://" + adminAddr + path) // no session needed
	if err != nil {
		e.t.Fatal(err)
	}
	script, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		e.t.Fatalf("bootstrap link: %d %s", resp.StatusCode, script)
	}
	out, errOut, code := e.exec(container, script, append([]string{"sh", "-s", "--"}, flags...)...)
	return out + errOut, code
}

// mintCert signs a certificate directly with the CA key, bypassing
// SignUserCert's rules so tests can build expired or mis-targeted certs.
func (e *env) mintCert(principal string, src string, validBefore time.Time) ssh.Signer {
	e.t.Helper()
	_, priv, _ := ed25519GenerateKey()
	signer, _ := ssh.NewSignerFromKey(priv)
	cert := &ssh.Certificate{
		Key: signer.PublicKey(), Serial: uint64(time.Now().UnixNano()), CertType: ssh.UserCert, KeyId: "lor-integration",
		ValidPrincipals: []string{principal},
		ValidAfter:      uint64(time.Now().Add(-10 * time.Minute).Unix()),
		ValidBefore:     uint64(validBefore.Unix()),
		Permissions: ssh.Permissions{
			CriticalOptions: map[string]string{"source-address": src + "/32"},
			Extensions:      map[string]string{"permit-pty": ""},
		},
	}
	if err := cert.SignCert(rand.Reader, e.caSigner); err != nil {
		e.t.Fatal(err)
	}
	cs, err := ssh.NewCertSigner(cert, signer)
	if err != nil {
		e.t.Fatal(err)
	}
	return cs
}

// sshRun logs in from the runner (172.30.99.1) and runs a command.
func sshRun(ip, user string, signer ssh.Signer, cmd string) (string, error) {
	client, err := ssh.Dial("tcp", ip+":22", &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second,
	})
	if err != nil {
		return "", err
	}
	defer client.Close()
	s, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	out, err := s.CombinedOutput(cmd)
	return strings.TrimSpace(string(out)), err
}

func sshClient(ip, user string, signer ssh.Signer) (*ssh.Client, error) {
	return ssh.Dial("tcp", ip+":22", &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second,
	})
}
