package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"90d":    90 * 24 * time.Hour,
		"1w":     7 * 24 * time.Hour,
		"1w2d":   9 * 24 * time.Hour,
		"720h":   720 * time.Hour,
		"1h30m":  90 * time.Minute,
		"1d12h":  36 * time.Hour,
		"15m":    15 * time.Minute,
		"1.5h":   90 * time.Minute,
		"2d1.5h": 49*time.Hour + 30*time.Minute,
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "1x", "-1h", "h1", "213504d", "1.5d", "999999999999w", "106751d106751d"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) should fail", bad)
		}
	}
}

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lor-server.toml")
	os.WriteFile(p, []byte(`
data_dir = "/var/lib/lor"
public_url = "https://lor.home.example.com/"
admin_listen = "192.168.1.10:8444"
trusted_proxy = "192.168.1.30"
agent_listen = "192.168.1.10:8443"
command_log_retention = "90d"
`), 0o600)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://lor.home.example.com" || cfg.RPID != "lor.home.example.com" {
		t.Errorf("public url/rpid = %q/%q", cfg.PublicURL, cfg.RPID)
	}
	if cfg.CommandLogRetention.D() != 90*24*time.Hour || cfg.AgentRequestMax.D() != 8*time.Hour {
		t.Errorf("durations wrong: %v %v", cfg.CommandLogRetention, cfg.AgentRequestMax)
	}
	if cfg.TrustedProxyIP.String() != "192.168.1.30" {
		t.Errorf("trusted proxy = %v", cfg.TrustedProxyIP)
	}
}

func TestLoadRejects(t *testing.T) {
	base := `
data_dir = "/var/lib/lor"
admin_listen = "192.168.1.10:8444"
agent_listen = "192.168.1.10:8443"
`
	cases := map[string]string{
		"http public url":   base + `public_url = "http://lor.example.com"` + "\ntrusted_proxy = \"192.168.1.30\"",
		"proxy hostname":    base + `public_url = "https://lor.example.com"` + "\ntrusted_proxy = \"caddy.lan\"",
		"unknown key":       base + `public_url = "https://lor.example.com"` + "\ntrusted_proxy = \"192.168.1.30\"\ndev = true",
		"agent max > abs":   base + `public_url = "https://lor.example.com"` + "\ntrusted_proxy = \"192.168.1.30\"\nagent_request_max = \"1000h\"",
		"public url w/path": base + `public_url = "https://lor.example.com/lor"` + "\ntrusted_proxy = \"192.168.1.30\"",
		"public url frag":   base + `public_url = "https://lor.example.com/#admin"` + "\ntrusted_proxy = \"192.168.1.30\"",
		"public url query":  base + `public_url = "https://lor.example.com/?x=1"` + "\ntrusted_proxy = \"192.168.1.30\"",
		"public url user":   base + `public_url = "https://a@lor.example.com"` + "\ntrusted_proxy = \"192.168.1.30\"",
	}
	for name, content := range cases {
		p := filepath.Join(t.TempDir(), "c.toml")
		os.WriteFile(p, []byte(content), 0o600)
		if _, err := Load(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
