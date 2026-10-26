package grants

import (
	"fmt"

	"github.com/joshualent/lor/internal/hosts"
	"github.com/joshualent/lor/internal/store"
)

// Warning is a server-computed danger notice. The UI shows the warnings whose
// Levels include the level being granted; it never decides what's dangerous.
type Warning struct {
	Code     string
	Severity string // danger | warning | info
	Levels   []string
	Title    string
	Message  string
}

// HostWarnings returns the warnings for granting access to h.
func HostWarnings(h *store.Host) []Warning {
	var out []Warning
	if h.LorInfra {
		// Strongest warning, at any level, listed first.
		out = append(out, Warning{
			Code: "lor_infra", Severity: "danger", Levels: []string{LevelUser, LevelSudo},
			Title: fmt.Sprintf("%s is Lor infrastructure", h.Name),
			Message: fmt.Sprintf("%s runs Lor or proxies its web UI. Root here can read Lor's signing key and give itself "+
				"access to every host, or change what you see on approval pages. Keep this grant short; if a sudo session "+
				"here is ever in doubt, rotate the CA.", h.Name),
		})
	}
	if h.MaxLevel == LevelSudo {
		out = append(out, Warning{
			Code: "root_access", Severity: "danger", Levels: []string{LevelSudo},
			Title:   "Root access (sudo)",
			Message: fmt.Sprintf("Logs in as agent-sudo. Can read, change or delete anything on %s.", h.Name),
		})
		if h.Hypervisor {
			out = append(out, Warning{
				Code: "hypervisor", Severity: "danger", Levels: []string{LevelSudo},
				Title:   fmt.Sprintf("%s is a hypervisor", h.Name),
				Message: fmt.Sprintf("Sudo on %s controls every container and VM on it.", h.Name),
			})
		}
	}
	return out
}

// Limits describes what a Lor certificate login can't do, for the consent
// page. The idle-timeout line depends on the host's OpenSSH version (see
// hosts.IdleTimeout): before 10.6 Lor's 15-minute idle timeout is written
// but not enforced.
func Limits(agent *store.Agent, host *store.Host) []string {
	from := "any registered address"
	if len(agent.LANAddrs) > 0 {
		from = fmt.Sprintf("only %s", joinAddrs(agent.LANAddrs))
	}
	idle := "Idle timeout depends on the host's OpenSSH (enforced from 10.6); rescan the host to check"
	switch hosts.IdleTimeout(host.SSHVersion) {
	case "enforced":
		idle = "Idle sessions close after 15 min"
	case "unenforced":
		idle = fmt.Sprintf("No idle timeout: %s doesn't enforce it (needs OpenSSH 10.6+); open sessions last until closed", sshName(host.SSHVersion))
	}
	return []string{
		"Certificate login only",
		"No port, agent or X11 forwarding",
		idle,
		"Connect from " + from,
	}
}

func sshName(v string) string {
	if major, minor, ok := hosts.OpenSSHVersion(v); ok {
		return fmt.Sprintf("OpenSSH %d.%d", major, minor)
	}
	return "this OpenSSH"
}

func joinAddrs(a []string) string {
	s := ""
	for i, x := range a {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}
