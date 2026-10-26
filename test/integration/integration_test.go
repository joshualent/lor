//go:build integration

package integration

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func ed25519GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

const reason = "Integration test: check the service status and recent logs."

var linkRe = regexp.MustCompile(`https://lor\.test/r/(\d+)`)

func TestIntegration(t *testing.T) {
	e := newEnv(t)

	// ---- setup: hosts, agent, bootstrap
	var hostIDs = map[string]int64{}
	for name, ip := range map[string]string{"nas": nasIP, "pihole": piholeIP, "rb": rbIP} {
		var d struct {
			Host struct {
				ID          int64  `json:"id"`
				Fingerprint string `json:"host_key_fingerprint"`
			} `json:"host"`
			ScanError string `json:"scan_error"`
		}
		e.post("/hosts", map[string]any{"name": name, "address": ip, "max_level": "sudo"}, &d)
		if d.ScanError != "" || d.Host.Fingerprint == "" {
			t.Fatalf("host %s: scan failed: %s", name, d.ScanError)
		}
		hostIDs[name] = d.Host.ID
	}
	var en struct {
		Agent   struct{ ID int64 } `json:"agent"`
		Command string             `json:"command"`
	}
	e.post("/agents", map[string]any{"name": "claude", "lan_addrs": []string{agentIP}}, &en)
	agentID := en.Agent.ID
	if out, errOut, code := e.lor(strings.Fields(en.Command)[1:]...); code != 0 {
		t.Fatalf("enroll: %d %s %s", code, out, errOut)
	}

	for _, h := range []struct{ c, ip string }{{"nas", nasIP}, {"pihole", piholeIP}} {
		out, code := e.bootstrap(h.c, h.c)
		if code != 0 {
			t.Fatalf("bootstrap %s: exit %d\n%s", h.c, code, out)
		}
		t.Logf("bootstrap %s:\n%s", h.c, out)
		// The script prints the host's fingerprint; it must match Lor's scan.
		var hd struct {
			Host struct {
				Fingerprint string `json:"host_key_fingerprint"`
			} `json:"host"`
		}
		e.get("/hosts/"+h.c, &hd)
		if !strings.Contains(out, hd.Host.Fingerprint) {
			t.Errorf("bootstrap output doesn't show Lor's fingerprint %s", hd.Host.Fingerprint)
		}
	}
	if strings.HasPrefix(e.distro, "ubuntu:26") || strings.HasPrefix(e.distro, "ubuntu:25") {
		out, _, _ := e.exec("nas", nil, "sudo", "--version")
		if !strings.Contains(strings.ToLower(out), "sudo-rs") {
			t.Errorf("expected sudo-rs on %s, got %q", e.distro, out)
		}
	}

	hour := time.Now().Add(time.Hour)

	// ---- 1. A valid certificate logs in as agent.
	t.Run("01_valid_cert_logs_in_as_agent", func(t *testing.T) {
		out, err := sshRun(nasIP, "agent", e.mintCert("nas", gateway, hour), "id -un")
		if err != nil || out != "agent" {
			t.Fatalf("login: %q %v", out, err)
		}
	})

	// ---- 2. Wrong host: a certificate for nas does nothing on pihole.
	t.Run("02_wrong_principal_rejected", func(t *testing.T) {
		if _, err := sshRun(piholeIP, "agent", e.mintCert("nas", gateway, hour), "true"); err == nil {
			t.Fatal("nas certificate accepted on pihole")
		}
	})

	// ---- 3. A user-level certificate is rejected for agent-sudo.
	t.Run("03_user_cert_rejected_for_agent_sudo", func(t *testing.T) {
		if _, err := sshRun(nasIP, "agent-sudo", e.mintCert("nas", gateway, hour), "true"); err == nil {
			t.Fatal("user certificate accepted for agent-sudo")
		}
	})

	// ---- 4. Wrong source IP.
	t.Run("04_wrong_source_ip_rejected", func(t *testing.T) {
		if _, err := sshRun(nasIP, "agent", e.mintCert("nas", agentIP, hour), "true"); err == nil {
			t.Fatal("certificate for 172.30.99.10 accepted from 172.30.99.1")
		}
	})

	// ---- 5. Expired certificate.
	t.Run("05_expired_cert_rejected", func(t *testing.T) {
		if _, err := sshRun(nasIP, "agent", e.mintCert("nas", gateway, time.Now().Add(-time.Minute)), "true"); err == nil {
			t.Fatal("expired certificate accepted")
		}
	})

	// ---- 6. A planted authorized_keys entry is ignored.
	t.Run("06_planted_key_ignored", func(t *testing.T) {
		_, priv, _ := ed25519GenerateKey()
		signer, _ := ssh.NewSignerFromKey(priv)
		pub := string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
		_, errOut, code := e.exec("nas", []byte(pub), "sh", "-c",
			"mkdir -p /home/agent/.ssh && cat > /home/agent/.ssh/authorized_keys && chown -R agent /home/agent/.ssh && chmod 700 /home/agent/.ssh && chmod 600 /home/agent/.ssh/authorized_keys")
		if code != 0 {
			t.Fatal(errOut)
		}
		if _, err := sshRun(nasIP, "agent", signer, "true"); err == nil {
			t.Fatal("planted key accepted")
		}
	})

	// ---- 7. A Lor certificate can't log in as root or other users.
	t.Run("07_not_root_or_other_users", func(t *testing.T) {
		e.exec("nas", nil, "useradd", "-m", "-s", "/bin/bash", "alice")
		e.exec("nas", nil, "usermod", "-p", "*", "alice")
		for _, user := range []string{"root", "alice"} {
			for _, p := range []string{"nas", "nas:sudo", user} {
				if _, err := sshRun(nasIP, user, e.mintCert(p, gateway, hour), "true"); err == nil {
					t.Fatalf("certificate with principal %q logged in as %s", p, user)
				}
			}
		}
	})

	// ---- 8. Port forwarding is refused.
	t.Run("08_port_forwarding_refused", func(t *testing.T) {
		c, err := sshClient(nasIP, "agent", e.mintCert("nas", gateway, hour))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if conn, err := c.Dial("tcp", "127.0.0.1:22"); err == nil {
			conn.Close()
			t.Fatal("direct-tcpip (ssh -L) allowed")
		}
		if ln, err := c.Listen("tcp", "127.0.0.1:2222"); err == nil {
			ln.Close()
			t.Fatal("remote forwarding (ssh -R) allowed")
		}
	})

	// ---- 9. agent can't sudo; agent-sudo can.
	t.Run("09_sudo_only_for_agent_sudo", func(t *testing.T) {
		if out, err := sshRun(nasIP, "agent", e.mintCert("nas", gateway, hour), "sudo -n true"); err == nil {
			t.Fatalf("agent could sudo: %q", out)
		}
		out, err := sshRun(nasIP, "agent-sudo", e.mintCert("nas:sudo", gateway, hour), "sudo -n id -u")
		if err != nil || out != "0" {
			t.Fatalf("agent-sudo sudo: %q %v", out, err)
		}
	})

	// ---- 10. Bootstrap is idempotent; --uninstall makes 1 and 9 fail.
	t.Run("10_bootstrap_idempotent_and_uninstall", func(t *testing.T) {
		before, _, _ := e.exec("nas", nil, "sh", "-c", "cat /etc/ssh/sshd_config.d/50-lor.conf /etc/ssh/lor/principals/* /etc/sudoers.d/50-lor-agent; id agent; id agent-sudo")
		out, code := e.bootstrap("nas", "nas")
		if code != 0 {
			t.Fatalf("second run: %d\n%s", code, out)
		}
		after, _, _ := e.exec("nas", nil, "sh", "-c", "cat /etc/ssh/sshd_config.d/50-lor.conf /etc/ssh/lor/principals/* /etc/sudoers.d/50-lor-agent; id agent; id agent-sudo")
		if before != after {
			t.Fatalf("second run changed state:\n%s\n---\n%s", before, after)
		}
		if out, err := sshRun(nasIP, "agent", e.mintCert("nas", gateway, hour), "true"); err != nil {
			t.Fatalf("login after re-run: %q %v", out, err)
		}

		out, code = e.bootstrap("nas", "nas", "--uninstall")
		if code != 0 {
			t.Fatalf("uninstall: %d\n%s", code, out)
		}
		if _, err := sshRun(nasIP, "agent", e.mintCert("nas", gateway, hour), "true"); err == nil {
			t.Fatal("case 1 still works after --uninstall")
		}
		if _, err := sshRun(nasIP, "agent-sudo", e.mintCert("nas:sudo", gateway, hour), "sudo -n true"); err == nil {
			t.Fatal("case 9 still works after --uninstall")
		}
		if _, _, code := e.exec("nas", nil, "id", "agent"); code == 0 {
			t.Fatal("agent user still exists after --uninstall")
		}
		// Reinstall for the CLI cases.
		if out, code := e.bootstrap("nas", "nas"); code != 0 {
			t.Fatalf("reinstall: %d\n%s", code, out)
		}
		// --no-sudo on an existing host removes agent-sudo.
		if out, code := e.bootstrap("pihole", "pihole", "--no-sudo"); code != 0 {
			t.Fatalf("--no-sudo: %d\n%s", code, out)
		}
		if _, _, code := e.exec("pihole", nil, "id", "agent-sudo"); code == 0 {
			t.Fatal("agent-sudo still exists after --no-sudo")
		}
		if out, err := sshRun(piholeIP, "agent", e.mintCert("pihole", gateway, hour), "id -un"); err != nil || out != "agent" {
			t.Fatalf("agent on pihole after --no-sudo: %q %v", out, err)
		}
	})

	// ---- 11. Rollback on validation failure leaves sshd working.
	t.Run("11_rollback_on_validation_failure", func(t *testing.T) {
		// A pre-existing Match block that overrides Lor's AuthorizedKeysFile:
		// validation (sshd -T) must catch it and roll everything back.
		override := "Match User agent\n    AuthorizedKeysFile .ssh/authorized_keys\nMatch all\n"
		_, errOut, code := e.exec("rb", []byte(override), "sh", "-c", "cat > /etc/ssh/sshd_config.d/10-override.conf")
		if code != 0 {
			t.Fatal(errOut)
		}
		cfgBefore, _, _ := e.exec("rb", nil, "cat", "/etc/ssh/sshd_config")
		out, code := e.bootstrap("rb", "rb")
		if code == 0 {
			t.Fatalf("bootstrap should fail validation:\n%s", out)
		}
		if !strings.Contains(out, "validation failed") || !strings.Contains(out, "rolling back") {
			t.Fatalf("unexpected output:\n%s", out)
		}
		for _, f := range []string{"/etc/ssh/sshd_config.d/50-lor.conf", "/etc/ssh/lor", "/etc/sudoers.d/50-lor-agent"} {
			if _, _, code := e.exec("rb", nil, "test", "-e", f); code == 0 {
				t.Errorf("%s left behind after rollback", f)
			}
		}
		for _, u := range []string{"agent", "agent-sudo"} {
			if _, _, code := e.exec("rb", nil, "id", u); code == 0 {
				t.Errorf("user %s left behind after rollback", u)
			}
		}
		cfgAfter, _, _ := e.exec("rb", nil, "cat", "/etc/ssh/sshd_config")
		if cfgBefore != cfgAfter {
			t.Error("sshd_config changed after rollback")
		}
		if _, errOut, code := e.exec("rb", nil, "/usr/sbin/sshd", "-t"); code != 0 {
			t.Fatalf("sshd -t fails after rollback: %s", errOut)
		}
		waitSSH(t, rbIP)
	})

	// ---- 12. CLI request flow.
	t.Run("12_cli_request_flow", func(t *testing.T) {
		out, errOut, code := e.lor("request", "nas", "--for", "10m", "--reason", reason)
		if code != 10 || !linkRe.MatchString(out) {
			t.Fatalf("request: %d\n%s\n%s", code, out, errOut)
		}
		id := linkRe.FindStringSubmatch(out)[1]
		e.stepUp(map[string]any{"kind": "authorize_request", "grant_id": atoi(id), "level": "user", "duration_s": 600})
		out, errOut, code = e.lor("ssh", "nas", "--", "id", "-un")
		if code != 0 || strings.TrimSpace(out) != "agent" {
			t.Fatalf("lor ssh: %d %q %s", code, out, errOut)
		}
		var g struct {
			Commands []struct{ Argv []string } `json:"commands"`
		}
		e.get("/grants/"+id, &g)
		if len(g.Commands) == 0 || strings.Join(g.Commands[0].Argv, " ") != "id -un" {
			t.Fatalf("command log %+v", g.Commands)
		}
		// scp/rsync path: raw ssh with the generated config.
		out, errOut, code = e.exec("agent", nil, "sh", "-c", `HOME=/root ssh -F "$(/opt/lor/lor config-path)" lor-nas hostname`)
		if code != 0 || strings.TrimSpace(out) != "nas" {
			t.Fatalf("config-path ssh: %d %q %s", code, out, errOut)
		}
		if _, _, code := e.lor("release", "nas"); code != 0 {
			t.Fatal("release failed")
		}
	})

	// ---- 13. Offer flow.
	t.Run("13_offer_flow", func(t *testing.T) {
		e.stepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostIDs["pihole"], "level": "user", "duration_s": 600})
		out, errOut, code := e.lor("ssh", "pihole", "--", "hostname")
		if code != 0 || strings.TrimSpace(out) != "pihole" || !strings.Contains(errOut, "claimed the offer") {
			t.Fatalf("offer claim: %d %q %s", code, out, errOut)
		}
		e.lor("release", "pihole")
	})

	// ---- 14. Downgrade: sudo requested, user approved.
	t.Run("14_downgrade", func(t *testing.T) {
		out, errOut, code := e.lor("request", "nas", "--sudo", "--for", "10m", "--reason", reason)
		if code != 10 {
			t.Fatalf("request: %d %s %s", code, out, errOut)
		}
		id := linkRe.FindStringSubmatch(out)[1]
		e.stepUp(map[string]any{"kind": "authorize_request", "grant_id": atoi(id), "level": "user", "duration_s": 600})
		out, _, code = e.lor("ssh", "nas", "--", "id", "-un")
		if code != 0 || strings.TrimSpace(out) != "agent" {
			t.Fatalf("expected agent, got %d %q", code, out)
		}
		if _, _, code := e.lor("ssh", "nas", "--", "sudo", "-n", "true"); code == 0 {
			t.Fatal("sudo worked after a downgrade")
		}
		e.lor("release", "nas")
	})

	// ---- 16. Revoked grant: lor ssh deletes the key and refuses.
	t.Run("16_revoked", func(t *testing.T) {
		res := e.stepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostIDs["nas"], "level": "sudo", "duration_s": 600, "confirm_sudo": true})
		gid := int64(res["grant"].(map[string]any)["id"].(float64))
		if out, errOut, code := e.lor("ssh", "nas", "--", "sudo", "-n", "id", "-u"); code != 0 || strings.TrimSpace(out) != "0" {
			t.Fatalf("sudo grant: %d %q %s", code, out, errOut)
		}
		e.post(fmt.Sprintf("/grants/%d/revoke", gid), map[string]any{}, nil)
		_, errOut, code := e.lor("ssh", "nas", "--", "true")
		if code != 255 || !strings.Contains(errOut, "revoked") {
			t.Fatalf("after revoke: %d %s", code, errOut)
		}
		if _, _, code := e.exec("agent", nil, "test", "-e", fmt.Sprintf("/root/.config/lor/grants/%d", gid)); code == 0 {
			t.Fatal("key directory still exists after revoke")
		}
	})

	// ---- 17. Exit codes.
	t.Run("17_exit_codes", func(t *testing.T) {
		e.stepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostIDs["nas"], "level": "user", "duration_s": 600})
		if _, _, code := e.lor("ssh", "nas", "--", "sh", "-c", "exit 7"); code != 7 {
			t.Fatalf("remote exit code: got %d, want 7", code)
		}
		if _, errOut, code := e.lor("ssh", "nohost", "--", "true"); code != 255 || !strings.HasPrefix(errOut, "lor: ") {
			t.Fatalf("lor-side failure: %d %q", code, errOut)
		}
		e.lor("release", "nas")
	})

	// ---- 15. Extension in the last 30s keeps access past the original end;
	// without one, access ends (sshd enforces expiry even for a saved key).
	t.Run("15_extension_in_last_30s", func(t *testing.T) {
		res := e.stepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostIDs["nas"], "level": "user", "duration_s": 75})
		start := time.Now()
		gid := int64(res["grant"].(map[string]any)["id"].(float64))
		res2 := e.stepUp(map[string]any{"kind": "create_offer", "agent_id": agentID, "host_id": hostIDs["pihole"], "level": "user", "duration_s": 75})
		gid2 := int64(res2["grant"].(map[string]any)["id"].(float64))
		for _, h := range []string{"nas", "pihole"} {
			if out, errOut, code := e.lor("ssh", h, "--", "true"); code != 0 {
				t.Fatalf("claim %s: %d %s %s", h, code, out, errOut)
			}
		}
		// Keep a copy of pihole's key and certificate to try raw ssh after expiry.
		e.exec("agent", nil, "sh", "-c", fmt.Sprintf("mkdir -p /tmp/saved && cp /root/.config/lor/grants/%d/id_ed25519* /tmp/saved/", gid2))

		time.Sleep(time.Until(start.Add(50 * time.Second))) // ~25s left
		out, errOut, code := e.lor("extend", "nas", "--for", "5m", "--reason", "Still checking the logs, need a few more minutes.")
		if code != 10 {
			t.Fatalf("extend: %d %s %s", code, out, errOut)
		}
		var g struct {
			PendingExtension struct{ ID int64 } `json:"pending_extension"`
		}
		e.get(fmt.Sprintf("/grants/%d", gid), &g)
		e.stepUp(map[string]any{"kind": "approve_extension", "extension_id": g.PendingExtension.ID, "duration_s": 300})

		time.Sleep(time.Until(start.Add(85 * time.Second))) // past the original end
		if out, errOut, code := e.lor("ssh", "nas", "--", "true"); code != 0 {
			t.Fatalf("extended grant stopped working: %d %s %s", code, out, errOut)
		}
		if _, errOut, code := e.lor("ssh", "pihole", "--", "true"); code != 255 {
			t.Fatalf("unextended grant still works through lor: %d %s", code, errOut)
		}
		// sshd itself rejects the expired certificate.
		_, errOut, code = e.exec("agent", nil, "ssh", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
			"-o", "IdentitiesOnly=yes", "-i", "/tmp/saved/id_ed25519", "-o", "CertificateFile=/tmp/saved/id_ed25519-cert.pub",
			"agent@"+piholeIP, "true")
		if code == 0 {
			t.Fatal("expired certificate accepted by sshd")
		}
		t.Logf("raw ssh with expired cert: exit %d (%s)", code, strings.TrimSpace(errOut))
		e.lor("release", "nas")
	})
}

func atoi(s string) int64 {
	var n int64
	fmt.Sscan(s, &n)
	return n
}
