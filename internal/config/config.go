// Package config loads lor-server.toml (plan section 15).
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration is a time.Duration that also accepts "d" (day) and "w" (week)
// units, e.g. "90d" or "1w2d".
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// D returns the duration as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// ParseDuration parses Go durations plus "d" and "w" units.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty duration")
	}
	var total time.Duration
	rest := s
	for rest != "" {
		i := 0
		for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9') {
			i++
		}
		if i == 0 {
			// Not a d/w prefix; hand the remainder to time.ParseDuration.
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		j := i
		for j < len(rest) && !(rest[j] >= '0' && rest[j] <= '9') {
			j++
		}
		num, unit := rest[:i], rest[i:j]
		n, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		switch unit {
		case "d":
			total += time.Duration(n) * 24 * time.Hour
		case "w":
			total += time.Duration(n) * 7 * 24 * time.Hour
		default:
			v, err := time.ParseDuration(num + unit)
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q", s)
			}
			total += v
		}
		rest = rest[j:]
	}
	return total, nil
}

// Config is lor-server's configuration.
type Config struct {
	DataDir                string   `toml:"data_dir"`
	PublicURL              string   `toml:"public_url"`
	AdminListen            string   `toml:"admin_listen"`
	TrustedProxy           string   `toml:"trusted_proxy"`
	AgentListen            string   `toml:"agent_listen"`
	DefaultGrantDuration   Duration `toml:"default_grant_duration"`
	AgentRequestMax        Duration `toml:"agent_request_max"`
	AbsoluteMaxDuration    Duration `toml:"absolute_max_duration"`
	RequestApprovalTimeout Duration `toml:"request_approval_timeout"`
	CommandLogRetention    Duration `toml:"command_log_retention"`
	// TLSExtraSANs adds names or IPs to the self-signed TLS certificate, on top
	// of the admin and agent listen IPs.
	TLSExtraSANs []string `toml:"tls_extra_sans"`

	// Derived, not read from the file.
	PublicOrigin   string           `toml:"-"`
	RPID           string           `toml:"-"`
	TrustedProxyIP netip.Addr       `toml:"-"`
	ExtraOrigins   []string         `toml:"-"` // dev only
	Dev            bool             `toml:"-"` // command-line flag only
	AdminTLS       bool             `toml:"-"` // false only in dev
	Now            func() time.Time `toml:"-"`
}

// Default returns a config with the defaults from plan section 15.
func Default() Config {
	return Config{
		DataDir:                "/var/lib/lor",
		DefaultGrantDuration:   Duration(time.Hour),
		AgentRequestMax:        Duration(8 * time.Hour),
		AbsoluteMaxDuration:    Duration(720 * time.Hour),
		RequestApprovalTimeout: Duration(30 * time.Minute),
		CommandLogRetention:    Duration(90 * 24 * time.Hour),
		AdminTLS:               true,
	}
}

// Load reads and validates a config file.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return cfg, fmt.Errorf("config %s: unknown keys %v", path, und)
	}
	return cfg, cfg.Finish()
}

// Finish validates the config and fills in derived fields.
func (c *Config) Finish() error {
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.DataDir == "" {
		return errors.New("config: data_dir is required")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(c.Dev && u.Scheme == "http")) {
		return fmt.Errorf("config: public_url must be an https URL, got %q", c.PublicURL)
	}
	if u.Path != "" && u.Path != "/" {
		return errors.New("config: public_url must not have a path")
	}
	c.PublicURL = strings.TrimSuffix(c.PublicURL, "/")
	c.PublicOrigin = u.Scheme + "://" + u.Host
	c.RPID = u.Hostname()

	if c.AdminListen == "" || c.AgentListen == "" {
		return errors.New("config: admin_listen and agent_listen are required")
	}
	for _, l := range []string{c.AdminListen, c.AgentListen} {
		if _, _, err := net.SplitHostPort(l); err != nil {
			return fmt.Errorf("config: invalid listen address %q: %w", l, err)
		}
	}
	ip, err := netip.ParseAddr(c.TrustedProxy)
	if err != nil {
		return fmt.Errorf("config: trusted_proxy must be an IP address, got %q", c.TrustedProxy)
	}
	c.TrustedProxyIP = ip.Unmap()

	for name, d := range map[string]Duration{
		"default_grant_duration":   c.DefaultGrantDuration,
		"agent_request_max":        c.AgentRequestMax,
		"absolute_max_duration":    c.AbsoluteMaxDuration,
		"request_approval_timeout": c.RequestApprovalTimeout,
		"command_log_retention":    c.CommandLogRetention,
	} {
		if d <= 0 {
			return fmt.Errorf("config: %s must be positive", name)
		}
	}
	if c.AgentRequestMax > c.AbsoluteMaxDuration {
		return errors.New("config: agent_request_max must not exceed absolute_max_duration")
	}
	if c.DefaultGrantDuration > c.AgentRequestMax {
		return errors.New("config: default_grant_duration must not exceed agent_request_max")
	}
	return nil
}

// AllowedOrigins lists the origins accepted by the CSRF check and WebAuthn.
func (c *Config) AllowedOrigins() []string {
	return append([]string{c.PublicOrigin}, c.ExtraOrigins...)
}
