package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/joshualent/lor/internal/cli"
	"github.com/joshualent/lor/internal/testenv"
)

type harness struct {
	t    *testing.T
	e    *testenv.Env
	url  string
	dir  string
	exec [][]string
}

func newHarness(t *testing.T) *harness {
	e := testenv.New(t)
	e.Setup()
	return &harness{t: t, e: e, url: e.StartAgentTLS(), dir: t.TempDir()}
}

func (h *harness) run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	app := &cli.App{
		State: &cli.State{Dir: h.dir, Now: h.e.Clock.Now},
		Out:   &out, Err: &errb, Now: h.e.Clock.Now, SSH: "sh",
		Exec: func(argv0 string, argv []string, env []string) error {
			h.exec = append(h.exec, argv)
			return nil
		},
	}
	code := app.Run(args)
	return code, out.String(), errb.String()
}

func (h *harness) enroll(name string) {
	h.t.Helper()
	r := h.e.AdminDo("POST", "/api/admin/v1/agents", map[string]any{"name": name, "lan_addrs": []string{"127.0.0.1"}})
	var en struct{ Command string }
	r.JSON(h.t, &en)
	code := en.Command[strings.LastIndex(en.Command, " ")+1:]
	if c, out, errs := h.run("enroll", "--url", h.url, "--fingerprint", h.e.TLS, "--code", code); c != 0 {
		h.t.Fatalf("enroll: %d %s %s", c, out, errs)
	}
	fi, err := os.Stat(filepath.Join(h.dir, "config.toml"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		h.t.Fatalf("config.toml mode: %v %v", fi, err)
	}
}

var linkRe = regexp.MustCompile(`https://lor\.example\.com/r/(\d+)`)

func TestRefusesWrongFingerprint(t *testing.T) {
	h := newHarness(t)
	r := h.e.AdminDo("POST", "/api/admin/v1/agents", map[string]any{"name": "claude", "lan_addrs": []string{"127.0.0.1"}})
	var en struct{ Command string }
	r.JSON(t, &en)
	code := en.Command[strings.LastIndex(en.Command, " ")+1:]
	c, _, errs := h.run("enroll", "--url", h.url, "--fingerprint", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "--code", code)
	if c == 0 || !strings.Contains(errs, "doesn't match the pinned") {
		t.Fatalf("wrong fingerprint accepted: %d %s", c, errs)
	}
	// After enrolling correctly, a changed pin in the config is refused too.
	h.enroll("claude2")
	cfgPath := filepath.Join(h.dir, "config.toml")
	b, _ := os.ReadFile(cfgPath)
	os.WriteFile(cfgPath, []byte(regexp.MustCompile(`fingerprint = ".*"`).ReplaceAllString(string(b), `fingerprint = "SHA256:BBBB"`)), 0o600)
	c, _, errs = h.run("status")
	if c == 0 || !strings.Contains(errs, "doesn't match the pinned") {
		t.Fatalf("status with wrong pin: %d %s", c, errs)
	}
}

func TestRequestApproveSSHRevoke(t *testing.T) {
	h := newHarness(t)
	h.e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	h.enroll("claude")

	c, out, errs := h.run("request", "nas", "--sudo", "--for", "1h", "--reason", "Check the ZFS pool status and SMART data.")
	if c != cli.ExitPending || !linkRe.MatchString(out) || !strings.Contains(out, "sudo (root) on nas for 1h") {
		t.Fatalf("request: %d\n%s\n%s", c, out, errs)
	}
	id := linkRe.FindStringSubmatch(out)[1]

	// Still pending: lor ssh prints the link again and exits 255.
	c, _, errs = h.run("ssh", "nas", "--", "uptime")
	if c != cli.ExitSSH || !strings.Contains(errs, "pending approval: https://lor.example.com/r/"+id) {
		t.Fatalf("ssh while pending: %d %s", c, errs)
	}

	// Approve with a downgrade.
	var gid int64
	json.Unmarshal([]byte(id), &gid)
	h.e.StepUp(map[string]any{"kind": "authorize_request", "grant_id": gid, "level": "user", "duration_s": 3600})

	c, _, errs = h.run("ssh", "nas", "--", "zpool", "status", "-v")
	if c != 0 || len(h.exec) != 1 {
		t.Fatalf("ssh: %d %s", c, errs)
	}
	argv := h.exec[0]
	if strings.Join(argv[1:], " ") != "-F "+filepath.Join(h.dir, "ssh_config")+" -- lor-nas zpool status -v" {
		t.Fatalf("ssh argv %q", argv)
	}
	cfg, _ := os.ReadFile(filepath.Join(h.dir, "ssh_config"))
	for _, want := range []string{"Host lor-nas", "HostName 192.168.1.20", "User agent\n", "IdentitiesOnly yes",
		"HostKeyAlias lor-host-nas", "StrictHostKeyChecking yes", "BatchMode yes", "grants/" + id + "/id_ed25519-cert.pub"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("ssh_config missing %q:\n%s", want, cfg)
		}
	}
	kh, _ := os.ReadFile(filepath.Join(h.dir, "known_hosts"))
	if !strings.HasPrefix(string(kh), "lor-host-nas ssh-ed25519 ") {
		t.Errorf("known_hosts %q", kh)
	}
	for _, f := range []string{"grants/" + id, "grants/" + id + "/id_ed25519"} {
		fi, err := os.Stat(filepath.Join(h.dir, f))
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if fi.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %o", f, fi.Mode().Perm())
		}
	}
	// The command was reported.
	var d struct {
		Commands []struct{ Argv []string }
	}
	h.e.AdminDo("GET", "/api/admin/v1/grants/"+id, nil).JSON(t, &d)
	if len(d.Commands) != 1 || strings.Join(d.Commands[0].Argv, " ") != "zpool status -v" {
		t.Fatalf("command log %+v", d.Commands)
	}

	// Short time left: warning.
	h.e.Clock.Advance(56 * time.Minute)
	_, _, errs = h.run("ssh", "nas", "--", "true")
	if !strings.Contains(errs, "warning: access to nas ends in") || !strings.Contains(errs, "lor extend nas") {
		t.Fatalf("no short-time warning: %s", errs)
	}

	// Revoke: lor ssh deletes the key and refuses.
	h.e.AdminDo("POST", "/api/admin/v1/grants/"+id+"/revoke", map[string]any{})
	n := len(h.exec)
	c, _, errs = h.run("ssh", "nas", "--", "id")
	if c != cli.ExitSSH || len(h.exec) != n || !strings.Contains(errs, "revoked") {
		t.Fatalf("ssh after revoke: %d %s", c, errs)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "grants", id)); !os.IsNotExist(err) {
		t.Fatal("key not deleted after revoke")
	}
}

func TestOfferClaimedBySSH(t *testing.T) {
	h := newHarness(t)
	hostID := h.e.AddHost("pihole", "192.168.1.21", "user", false, false)
	h.enroll("claude")
	var agents struct{ Agents []struct{ ID int64 } }
	h.e.AdminDo("GET", "/api/admin/v1/agents", nil).JSON(t, &agents)
	h.e.StepUp(map[string]any{"kind": "create_offer", "agent_id": agents.Agents[0].ID, "host_id": hostID, "level": "user", "duration_s": 1800})

	c, out, _ := h.run("status")
	if c != 0 || !strings.Contains(out, "offered") {
		t.Fatalf("status: %s", out)
	}
	c, _, errs := h.run("ssh", "pihole", "--", "pihole", "status")
	if c != 0 || len(h.exec) != 1 || !strings.Contains(errs, "claimed the offer") {
		t.Fatalf("ssh claim: %d %s", c, errs)
	}
	c, out, _ = h.run("release", "pihole")
	if c != 0 || !strings.Contains(out, "Released") {
		t.Fatalf("release: %d %s", c, out)
	}
	c, _, errs = h.run("ssh", "pihole", "--", "id")
	if c != cli.ExitSSH || !strings.Contains(errs, "no access to pihole") {
		t.Fatalf("after release: %d %s", c, errs)
	}
}

func TestRequestWaitDenied(t *testing.T) {
	h := newHarness(t)
	h.e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	h.enroll("claude")
	go func() {
		time.Sleep(300 * time.Millisecond)
		h.e.AdminDo("POST", "/api/admin/v1/grants/1/deny", map[string]any{"note": "not today"})
	}()
	c, _, errs := h.run("request", "nas", "--reason", "Check the ZFS pool status and SMART data.", "--wait")
	if c != cli.ExitDenied || !strings.Contains(errs, "not today") {
		t.Fatalf("wait denied: %d %s", c, errs)
	}
}

func TestInvalidRequestExitCode(t *testing.T) {
	h := newHarness(t)
	h.e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	h.enroll("claude")
	c, _, errs := h.run("request", "nas", "--reason", "too short")
	if c != cli.ExitInvalid || !strings.Contains(errs, "reason") {
		t.Fatalf("short reason: %d %s", c, errs)
	}
	c, _, _ = h.run("request", "nas", "--for", "nonsense", "--reason", "Check the ZFS pool status and SMART data.")
	if c != cli.ExitInvalid {
		t.Fatalf("bad duration: %d", c)
	}
	if c, _, _ := h.run("ssh", "nope", "--", "id"); c != cli.ExitSSH {
		t.Fatalf("unknown host ssh: %d", c)
	}
}

// An extension approved while the CLI was idle must survive the local
// certificate's expiry: lor ssh fetches the new certificate.
func TestExtendedWhileIdle(t *testing.T) {
	h := newHarness(t)
	h.e.AddHost("nas", "192.168.1.20", "sudo", false, false)
	h.enroll("claude")
	c, out, _ := h.run("request", "nas", "--for", "1h", "--reason", "Check the ZFS pool status and SMART data.")
	if c != cli.ExitPending {
		t.Fatalf("request: %d %s", c, out)
	}
	id := linkRe.FindStringSubmatch(out)[1]
	var gid int64
	json.Unmarshal([]byte(id), &gid)
	h.e.StepUp(map[string]any{"kind": "authorize_request", "grant_id": gid, "level": "user", "duration_s": 3600})
	if c, _, errs := h.run("ssh", "nas", "--", "true"); c != 0 {
		t.Fatalf("ssh: %d %s", c, errs)
	}
	h.e.StepUp(map[string]any{"kind": "extend_grant", "grant_id": gid, "duration_s": 3600})
	h.e.Clock.Advance(61 * time.Minute) // the local certificate has expired
	if c, _, errs := h.run("ssh", "nas", "--", "true"); c != 0 {
		t.Fatalf("ssh after extension: %d %s", c, errs)
	}
	// Once the grant really ends, the next run deletes the key.
	h.e.Clock.Advance(61 * time.Minute)
	h.run("status")
	if _, err := os.Stat(filepath.Join(h.dir, "grants", id)); !os.IsNotExist(err) {
		t.Fatal("expired grant directory not deleted")
	}
}
