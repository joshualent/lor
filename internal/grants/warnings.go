package grants

import (
	"fmt"

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
// page.
func Limits(agent *store.Agent) []string {
	from := "any registered address"
	if len(agent.LANAddrs) > 0 {
		from = fmt.Sprintf("only %s", joinAddrs(agent.LANAddrs))
	}
	return []string{
		"Certificate login only",
		"No port, agent or X11 forwarding",
		"Idle sessions close after 15 min (OpenSSH 9.2+)",
		"Connect from " + from,
	}
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
