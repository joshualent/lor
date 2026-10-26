// Package cli implements the `lor` agent CLI (plan section 10).
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/joshualent/lor/internal/client"
	"github.com/joshualent/lor/internal/config"
	"github.com/joshualent/lor/internal/tlscert"
	"github.com/joshualent/lor/internal/version"
)

// Exit codes (plan section 10). `lor ssh` returns the remote exit code, or
// 255 if lor stops before connecting.
const (
	ExitOK      = 0
	ExitError   = 1
	ExitDenied  = 2
	ExitExpired = 3
	ExitInvalid = 4
	ExitPending = 10
	ExitSSH     = 255
)

// CodeError carries an exit code and a message for stderr.
type CodeError struct {
	Code int
	Msg  string
}

func (e *CodeError) Error() string { return e.Msg }

func exitf(code int, format string, args ...any) error {
	return &CodeError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// App holds what commands share.
type App struct {
	State  *State
	Out    io.Writer
	Err    io.Writer
	Now    func() time.Time
	SSH    string                                                // ssh binary (default "ssh" from PATH)
	Exec   func(argv0 string, argv []string, env []string) error // syscall.Exec in production
	inSSH  bool
	client *client.ClientWithResponses
}

// NewApp returns an App using the default config directory.
func NewApp() (*App, error) {
	dir, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return &App{State: &State{Dir: dir, Now: time.Now}, Out: os.Stdout, Err: os.Stderr, Now: time.Now, SSH: "ssh", Exec: syscall.Exec}, nil
}

// Run executes the CLI and returns the process exit code.
func (a *App) Run(args []string) int {
	root := a.root()
	root.SetArgs(args)
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	var ee *CodeError
	if errors.As(err, &ee) {
		if ee.Msg != "" {
			fmt.Fprintln(a.Err, "lor:", ee.Msg)
		}
		return ee.Code
	}
	fmt.Fprintln(a.Err, "lor:", err)
	if a.inSSH {
		return ExitSSH
	}
	return ExitError
}

func (a *App) root() *cobra.Command {
	root := &cobra.Command{
		Use:           "lor",
		Short:         "Temporary, approved SSH access through Lor",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			a.cleanup(cmd)
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(a.enrollCmd(), a.statusCmd(), a.hostsCmd(), a.requestCmd(), a.extendCmd(), a.sshCmd(), a.releaseCmd(), a.configPathCmd())
	return root
}

// cleanup deletes expired grant directories. A local certificate past its
// end may belong to a grant that was extended while the CLI wasn't running,
// so those are only deleted once Lor confirms the grant is no longer active.
func (a *App) cleanup(cmd *cobra.Command) {
	expired := a.State.Cleanup()
	if len(expired) == 0 || cmd.Name() == "enroll" {
		return
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()
	active, err := a.listGrants(ctx, "", []client.GrantStatus{client.GrantStatusActive})
	if err != nil {
		return // can't confirm; keep the keys for now
	}
	live := map[int64]bool{}
	for _, g := range active {
		live[g.Id] = true
	}
	for _, id := range expired {
		if !live[id] {
			a.State.RemoveGrant(id)
		}
	}
}

// ---------------------------------------------------------------- API client

func httpClient(fingerprint string, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		TLSClientConfig:     tlscert.PinnedConfig(fingerprint),
		Proxy:               nil, // never through a proxy: Lor checks the source IP
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func (a *App) api() (*client.ClientWithResponses, *Config, error) {
	cfg, err := a.State.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	if a.client == nil {
		token := cfg.Token
		a.client, err = client.NewClientWithResponses(strings.TrimSuffix(cfg.URL, "/")+"/api/v1",
			client.WithHTTPClient(httpClient(cfg.Fingerprint, 75*time.Second)),
			client.WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
				r.Header.Set("Authorization", "Bearer "+token)
				r.Header.Set("User-Agent", "lor/"+version.Version)
				return nil
			}))
		if err != nil {
			return nil, nil, err
		}
	}
	return a.client, cfg, nil
}

// apiError is an error response from Lor.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Message }

func asAPIError(status int, body *client.Error, raw []byte) error {
	if body != nil {
		return &apiError{Status: status, Code: body.Error.Code, Message: body.Error.Message}
	}
	return &apiError{Status: status, Code: "http_" + fmt.Sprint(status), Message: fmt.Sprintf("unexpected response from Lor (HTTP %d): %.200s", status, raw)}
}

// exitFor maps an API error to the plan's exit codes.
func exitFor(err error) error {
	var ae *apiError
	if !errors.As(err, &ae) {
		return err
	}
	code := ExitError
	switch {
	case ae.Code == "grant_denied":
		code = ExitDenied
	case ae.Code == "grant_expired":
		code = ExitExpired
	case ae.Status == http.StatusBadRequest, ae.Status == http.StatusNotFound:
		code = ExitInvalid
	}
	return exitf(code, "%s", ae.Message)
}

// ---------------------------------------------------------------- enroll

func (a *App) enrollCmd() *cobra.Command {
	var u, fp, code string
	cmd := &cobra.Command{
		Use:   "enroll --url URL --fingerprint SHA256:… --code CODE",
		Short: "One-time setup (copy the command from Lor's Agents page)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pu, err := url.Parse(u)
			if err != nil || pu.Scheme != "https" || pu.Host == "" {
				return exitf(ExitInvalid, "--url must be https://<lor-lan-ip>:8443")
			}
			if !strings.HasPrefix(fp, "SHA256:") {
				return exitf(ExitInvalid, "--fingerprint must look like SHA256:…")
			}
			c, err := client.NewClientWithResponses(strings.TrimSuffix(u, "/")+"/api/v1", client.WithHTTPClient(httpClient(fp, 30*time.Second)))
			if err != nil {
				return err
			}
			res, err := c.EnrollWithResponse(cmd.Context(), client.EnrollJSONRequestBody{Code: code})
			if err != nil {
				return fmt.Errorf("can't reach Lor at %s: %w", u, err)
			}
			if res.JSON200 == nil {
				return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
			}
			if err := a.State.SaveConfig(&Config{URL: strings.TrimSuffix(u, "/"), Fingerprint: fp, Token: res.JSON200.Token, Agent: res.JSON200.Agent.Name}); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "Enrolled as %q. Config saved to %s.\n", res.JSON200.Agent.Name, a.State.path(configFile))
			return nil
		},
	}
	cmd.Flags().StringVar(&u, "url", "", "Lor agent API URL, e.g. https://192.168.1.10:8443")
	cmd.Flags().StringVar(&fp, "fingerprint", "", "Lor's TLS certificate fingerprint (pinned)")
	cmd.Flags().StringVar(&code, "code", "", "one-time enrollment code")
	for _, f := range []string{"url", "fingerprint", "code"} {
		cmd.MarkFlagRequired(f)
	}
	return cmd
}

// ---------------------------------------------------------------- status, hosts

func until(t *time.Time, now time.Time) string {
	if t == nil {
		return ""
	}
	left := t.Sub(now).Round(time.Minute)
	if left < time.Minute {
		left = t.Sub(now).Round(time.Second)
	}
	return fmt.Sprintf("until %s (%s left)", t.Local().Format("15:04"), formatDur(left))
}

func formatDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd%dh", d/(24*time.Hour), (d%(24*time.Hour))/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", d/time.Hour, (d%time.Hour)/time.Minute)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

func levelWord(l client.Level) string {
	if l == client.Sudo {
		return "sudo"
	}
	return "user"
}

func (a *App) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active, pending and offered grants",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			grants, err := a.listGrants(cmd.Context(), "", []client.GrantStatus{client.GrantStatusActive, client.GrantStatusPending, client.GrantStatusOffered})
			if err != nil {
				return err
			}
			if len(grants) == 0 {
				fmt.Fprintln(a.Out, "No access right now. Request it with: lor request <host> --for <duration> --reason \"...\"")
				return nil
			}
			now := a.Now()
			for _, g := range grants {
				switch g.Status {
				case client.GrantStatusActive:
					fmt.Fprintf(a.Out, "%-16s %-5s active  %s  (lor ssh %s -- <cmd>)\n", g.Host, levelWord(g.Level), until(g.ValidBefore, now), g.Host)
					if g.PendingExtension != nil {
						fmt.Fprintf(a.Out, "%-16s       extension pending: %s\n", "", deref(g.ApprovalUrl))
					}
				case client.GrantStatusOffered:
					fmt.Fprintf(a.Out, "%-16s %-5s offered %s  (lor ssh %s -- <cmd> claims it)\n", g.Host, levelWord(g.Level), until(g.ValidBefore, now), g.Host)
				case client.GrantStatusPending:
					fmt.Fprintf(a.Out, "%-16s %-5s pending approval: %s\n", g.Host, levelWord(g.Level), deref(g.ApprovalUrl))
				}
			}
			return nil
		},
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (a *App) hostsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hosts",
		Short: "List hosts you can request",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := a.api()
			if err != nil {
				return err
			}
			res, err := c.ListHostsWithResponse(cmd.Context())
			if err != nil {
				return fmt.Errorf("can't reach Lor: %w", err)
			}
			if res.JSON200 == nil {
				return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
			}
			for _, h := range res.JSON200.Hosts {
				fmt.Fprintf(a.Out, "%-20s max %s\n", h.Name, levelWord(h.MaxLevel))
			}
			fmt.Fprintf(a.Out, "Default duration %s; agents can ask for up to %s.\n",
				formatDur(time.Duration(res.JSON200.DefaultDurationS)*time.Second), formatDur(time.Duration(res.JSON200.AgentRequestMaxS)*time.Second))
			return nil
		},
	}
}

func (a *App) listGrants(ctx context.Context, host string, statuses []client.GrantStatus) ([]client.Grant, error) {
	c, _, err := a.api()
	if err != nil {
		return nil, err
	}
	p := &client.ListGrantsParams{}
	if host != "" {
		p.Host = &host
	}
	if len(statuses) > 0 {
		p.Status = &statuses
	}
	res, err := c.ListGrantsWithResponse(ctx, p)
	if err != nil {
		return nil, &netError{err}
	}
	if res.JSON200 == nil {
		return nil, exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
	}
	return res.JSON200.Grants, nil
}

// netError means Lor couldn't be reached.
type netError struct{ err error }

func (e *netError) Error() string { return "can't reach Lor: " + e.err.Error() }
func (e *netError) Unwrap() error { return e.err }

// ---------------------------------------------------------------- local state from API grants

func (a *App) local(g client.Grant) LocalGrant {
	lg := LocalGrant{ID: g.Id, Host: g.Host, Level: string(g.Level)}
	if g.Ssh != nil {
		lg.User, lg.Hostname, lg.Port, lg.Alias = string(g.Ssh.User), g.Ssh.Hostname, g.Ssh.Port, g.Ssh.HostKeyAlias
	}
	return lg
}

// install writes an active grant's certificate, known_hosts line and ssh_config.
func (a *App) install(g client.Grant) error {
	if g.Certificate == nil || g.Ssh == nil {
		return errors.New("Lor didn't return a certificate for an active grant")
	}
	if !a.State.HasKey(g.Id) {
		return exitf(ExitError, "the key for grant %d isn't on this machine (it was requested elsewhere); release it with `lor release %s` and request again", g.Id, g.Host)
	}
	if err := a.State.SaveCert(a.local(g), *g.Certificate); err != nil {
		return err
	}
	return a.State.WriteSSHConfig(g.Id, map[string]string{g.Ssh.HostKeyAlias: g.Ssh.KnownHostsLine})
}

func (a *App) printPending(g client.Grant, what string) {
	fmt.Fprintf(a.Out, "Approval needed: %s\n", deref(g.ApprovalUrl))
	fmt.Fprintf(a.Out, "Requested: %s\n", what)
	fmt.Fprintln(a.Out, "Share the link with the user and wait for them to approve it.")
}

func describe(host string, level client.Level, d time.Duration) string {
	dur := "the default duration"
	if d > 0 {
		dur = formatDur(d)
	}
	if level == client.Sudo {
		return fmt.Sprintf("sudo (root) on %s for %s", host, dur)
	}
	return fmt.Sprintf("standard user access on %s for %s", host, dur)
}

// ---------------------------------------------------------------- request

func (a *App) requestCmd() *cobra.Command {
	var sudo, wait bool
	var forStr, reason string
	cmd := &cobra.Command{
		Use:   "request <host> [--sudo] [--for 1h] --reason \"...\" [--wait]",
		Short: "Ask for access; prints an approval link and exits 10 (pending)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			level := client.User
			if sudo {
				level = client.Sudo
			}
			body := client.CreateGrantJSONRequestBody{Host: host, Level: level, Reason: reason}
			var dur time.Duration
			if forStr != "" {
				d, err := config.ParseDuration(forStr)
				if err != nil || d <= 0 {
					return exitf(ExitInvalid, "invalid --for %q (examples: 15m, 1h, 2h30m)", forStr)
				}
				dur = d
				s := int64(d / time.Second)
				body.DurationS = &s
			}
			c, _, err := a.api()
			if err != nil {
				return err
			}
			tmp, pub, err := a.State.NewKey()
			if err != nil {
				return err
			}
			body.PublicKey = pub
			res, err := c.CreateGrantWithResponse(cmd.Context(), body)
			if err != nil {
				os.RemoveAll(tmp)
				return fmt.Errorf("can't reach Lor: %w", err)
			}
			if res.JSON200 == nil {
				os.RemoveAll(tmp)
				return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
			}
			r := res.JSON200
			g := r.Grant
			if r.Reused != nil && *r.Reused {
				os.RemoveAll(tmp)
				if err := a.install(g); err != nil {
					return err
				}
				fmt.Fprintf(a.Out, "You already have %s access to %s %s. Use: lor ssh %s -- <cmd>\n", levelWord(g.Level), host, until(g.ValidBefore, a.Now()), host)
				return nil
			}
			if err := a.State.AdoptKey(tmp, a.local(g)); err != nil {
				return err
			}
			if g.Status == client.GrantStatusActive {
				if err := a.install(g); err != nil {
					return err
				}
				fmt.Fprintf(a.Out, "Access granted: %s on %s %s. Use: lor ssh %s -- <cmd>\n", levelWord(g.Level), host, until(g.ValidBefore, a.Now()), host)
				return nil
			}
			what := describe(host, g.Level, dur)
			a.printPending(g, what)
			if !wait {
				return exitf(ExitPending, "")
			}
			return a.waitForDecision(cmd.Context(), g.Id, what)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&sudo, "sudo", false, "ask for root (logs in as agent-sudo)")
	f.StringVar(&forStr, "for", "", "duration, e.g. 30m or 1h (default: Lor's default)")
	f.StringVar(&reason, "reason", "", "1–3 sentences: what you will do and why (10–400 characters)")
	f.BoolVar(&wait, "wait", false, "wait for the decision instead of exiting with 10")
	cmd.MarkFlagRequired("reason")
	return cmd
}

// waitForDecision long-polls until the request is decided.
func (a *App) waitForDecision(ctx context.Context, id int64, what string) error {
	c, _, err := a.api()
	if err != nil {
		return err
	}
	wait := 60
	for {
		res, err := c.GetGrantWithResponse(ctx, id, &client.GetGrantParams{Wait: &wait})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(3 * time.Second)
			continue
		}
		if res.JSON200 == nil {
			return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
		}
		g := *res.JSON200
		switch g.Status {
		case client.GrantStatusPending:
			continue
		case client.GrantStatusActive:
			if err := a.install(g); err != nil {
				return err
			}
			changed := ""
			if g.Level != g.RequestedLevel {
				changed = " (the level was changed by the approver)"
			}
			fmt.Fprintf(a.Out, "Approved: %s on %s %s%s. Use: lor ssh %s -- <cmd>\n", levelWord(g.Level), g.Host, until(g.ValidBefore, a.Now()), changed, g.Host)
			return nil
		case client.GrantStatusDenied:
			a.State.RemoveGrant(id)
			note := ""
			if g.DecisionNote != nil {
				note = ": " + *g.DecisionNote
			}
			return exitf(ExitDenied, "the request for %s was denied%s", what, note)
		default:
			a.State.RemoveGrant(id)
			return exitf(ExitExpired, "the request for %s ended without approval (%s)", what, g.Status)
		}
	}
}

// ---------------------------------------------------------------- extend

func (a *App) activeGrant(ctx context.Context, host string) (*client.Grant, error) {
	grants, err := a.listGrants(ctx, host, []client.GrantStatus{client.GrantStatusActive})
	if err != nil {
		return nil, err
	}
	g := pickActive(grants, a.State)
	if g == nil {
		return nil, exitf(ExitInvalid, "no active grant for %s", host)
	}
	return g, nil
}

// pickActive prefers grants whose key is on this machine, then sudo, then
// the latest end.
func pickActive(grants []client.Grant, st *State) *client.Grant {
	var best *client.Grant
	score := func(g *client.Grant) (int, int64) {
		s := 0
		if st.HasKey(g.Id) {
			s += 2
		}
		if g.Level == client.Sudo {
			s++
		}
		var end int64
		if g.ValidBefore != nil {
			end = g.ValidBefore.Unix()
		}
		return s, end
	}
	for i := range grants {
		g := &grants[i]
		if g.Status != client.GrantStatusActive {
			continue
		}
		if best == nil {
			best = g
			continue
		}
		s1, e1 := score(g)
		s2, e2 := score(best)
		if s1 > s2 || (s1 == s2 && e1 > e2) {
			best = g
		}
	}
	return best
}

func (a *App) extendCmd() *cobra.Command {
	var forStr, reason string
	var wait bool
	cmd := &cobra.Command{
		Use:   "extend <host> --for 1h --reason \"...\" [--wait]",
		Short: "Ask for more time; prints an approval link and exits 10 (pending)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			d, err := config.ParseDuration(forStr)
			if err != nil || d <= 0 {
				return exitf(ExitInvalid, "invalid --for %q (examples: 30m, 1h)", forStr)
			}
			g, err := a.activeGrant(cmd.Context(), host)
			if err != nil {
				return err
			}
			c, _, err := a.api()
			if err != nil {
				return err
			}
			res, err := c.ExtendGrantWithResponse(cmd.Context(), g.Id, client.ExtendGrantJSONRequestBody{DurationS: int64(d / time.Second), Reason: reason})
			if err != nil {
				return fmt.Errorf("can't reach Lor: %w", err)
			}
			if res.JSON200 == nil {
				return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
			}
			ext := res.JSON200.ExtensionRequest
			fmt.Fprintf(a.Out, "Approval needed: %s\n", res.JSON200.ApprovalUrl)
			fmt.Fprintf(a.Out, "Requested: %s more on %s (%s), currently %s\n", formatDur(time.Duration(ext.RequestedDurationS)*time.Second), host, levelWord(g.Level), until(g.ValidBefore, a.Now()))
			fmt.Fprintln(a.Out, "Share the link with the user and wait for them to approve it.")
			if !wait {
				return exitf(ExitPending, "")
			}
			waitS := 60
			for {
				r, err := c.GetGrantWithResponse(cmd.Context(), g.Id, &client.GetGrantParams{Wait: &waitS})
				if err != nil {
					if cmd.Context().Err() != nil {
						return cmd.Context().Err()
					}
					time.Sleep(3 * time.Second)
					continue
				}
				if r.JSON200 == nil {
					return exitFor(asAPIError(r.StatusCode(), r.JSONDefault, r.Body))
				}
				ng := *r.JSON200
				x := ng.LatestExtension
				if x == nil || x.Id != ext.Id {
					return exitf(ExitError, "lost track of extension request %d", ext.Id)
				}
				switch x.Status {
				case client.ExtensionStatusPending:
					continue
				case client.ExtensionStatusApproved:
					if err := a.install(ng); err != nil {
						return err
					}
					fmt.Fprintf(a.Out, "Extended: %s %s\n", host, until(ng.ValidBefore, a.Now()))
					return nil
				case client.ExtensionStatusDenied:
					note := ""
					if x.DecisionNote != nil {
						note = ": " + *x.DecisionNote
					}
					return exitf(ExitDenied, "the extension was denied%s", note)
				default:
					return exitf(ExitExpired, "the extension request ended without approval (%s)", x.Status)
				}
			}
		},
	}
	cmd.Flags().StringVar(&forStr, "for", "", "additional time, e.g. 30m or 1h")
	cmd.Flags().StringVar(&reason, "reason", "", "why you need more time (10–400 characters)")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the decision instead of exiting with 10")
	cmd.MarkFlagRequired("for")
	cmd.MarkFlagRequired("reason")
	return cmd
}

// ---------------------------------------------------------------- release, config-path

func (a *App) releaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release <host>",
		Short: "Give up access to a host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			grants, err := a.listGrants(cmd.Context(), host, []client.GrantStatus{client.GrantStatusActive})
			if err != nil {
				return err
			}
			if len(grants) == 0 {
				return exitf(ExitError, "no active grant for %s", host)
			}
			c, _, _ := a.api()
			for _, g := range grants {
				res, err := c.ReleaseGrantWithResponse(cmd.Context(), g.Id)
				if err != nil {
					return fmt.Errorf("can't reach Lor: %w", err)
				}
				if res.JSON200 == nil {
					return exitFor(asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
				}
				a.State.RemoveGrant(g.Id)
				fmt.Fprintf(a.Out, "Released grant %d (%s on %s).\n", g.Id, levelWord(g.Level), host)
			}
			return a.State.WriteSSHConfig(0, nil)
		},
	}
}

func (a *App) configPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config-path",
		Short: "Print the generated ssh_config path (for scp/rsync: scp -F \"$(lor config-path)\" f lor-<host>:/tmp/)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := a.State.WriteSSHConfig(0, nil); err != nil {
				return err
			}
			fmt.Fprintln(a.Out, a.State.SSHConfigPath())
			return nil
		},
	}
}

// ---------------------------------------------------------------- ssh

func (a *App) sshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <host> -- <command...>",
		Short: "Run a command on a host (reported to Lor); claims an offer on first use",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.inSSH = true
			host := args[0]
			argv := args[1:]
			if cmd.ArgsLenAtDash() > 1 {
				return exitf(ExitSSH, "usage: lor ssh <host> -- <command...>")
			}
			return a.ssh(cmd.Context(), host, argv)
		},
	}
}

func (a *App) ssh(ctx context.Context, host string, argv []string) error {
	c, _, err := a.api()
	if err != nil {
		return exitf(ExitSSH, "%v", err)
	}
	grants, err := a.listGrants(ctx, host, nil)
	var ne *netError
	if errors.As(err, &ne) {
		return a.offline(host, argv, err)
	}
	if err != nil {
		return exitf(ExitSSH, "%v", err)
	}
	// Delete local keys of grants that ended (revoked, released, expired).
	ended := a.reconcile(grants)
	g := pickActive(grants, a.State)
	if g == nil {
		// Claim an offer on first use.
		for i := range grants {
			if grants[i].Status != client.GrantStatusOffered {
				continue
			}
			offer := grants[i]
			tmp, pub, err := a.State.NewKey()
			if err != nil {
				return exitf(ExitSSH, "%v", err)
			}
			res, err := c.ClaimGrantWithResponse(ctx, offer.Id, client.ClaimGrantJSONRequestBody{PublicKey: pub})
			if err != nil || res.JSON200 == nil {
				os.RemoveAll(tmp)
				if err != nil {
					return exitf(ExitSSH, "can't reach Lor: %v", err)
				}
				return exitf(ExitSSH, "claiming the offer failed: %v", asAPIError(res.StatusCode(), res.JSONDefault, res.Body))
			}
			claimed := *res.JSON200
			if err := a.State.AdoptKey(tmp, a.local(claimed)); err != nil {
				return exitf(ExitSSH, "%v", err)
			}
			fmt.Fprintf(a.Err, "lor: claimed the offer for %s (%s) %s\n", host, levelWord(claimed.Level), until(claimed.ValidBefore, a.Now()))
			g = &claimed
			break
		}
	}
	if g == nil {
		for _, p := range grants {
			if p.Status == client.GrantStatusPending {
				return exitf(ExitSSH, "the request for %s is still pending approval: %s", host, deref(p.ApprovalUrl))
			}
		}
		if ended != nil {
			return exitf(ExitSSH, "grant %d for %s was %s; the local key was deleted. %s", ended.Id, host, ended.Status, nextStep("grant_"+string(ended.Status), host))
		}
		return exitf(ExitSSH, "no access to %s; request it with: lor request %s --for <duration> --reason \"...\"", host, host)
	}
	if !a.State.HasKey(g.Id) {
		return exitf(ExitSSH, "the key for grant %d isn't on this machine; release it with `lor release %s` and request again", g.Id, host)
	}

	// Report the command and fetch the current certificate.
	if argv == nil {
		argv = []string{}
	}
	res, err := c.ConnectGrantWithResponse(ctx, g.Id, client.ConnectGrantJSONRequestBody{Argv: argv})
	if err != nil {
		return a.offline(host, argv, &netError{err})
	}
	if res.JSON200 == nil {
		ae := asAPIError(res.StatusCode(), res.JSONDefault, res.Body).(*apiError)
		switch ae.Code {
		case "grant_revoked", "grant_released", "grant_expired", "grant_denied", "grant_not_active":
			a.State.RemoveGrant(g.Id)
			a.State.WriteSSHConfig(0, nil)
			return exitf(ExitSSH, "%s; the local key was deleted. %s", ae.Message, nextStep(ae.Code, host))
		}
		return exitf(ExitSSH, "%s", ae.Message)
	}
	cur := *res.JSON200
	if err := a.install(cur); err != nil {
		return exitf(ExitSSH, "%v", err)
	}
	if cur.ValidBefore != nil {
		if left := cur.ValidBefore.Sub(a.Now()); left < 5*time.Minute {
			fmt.Fprintf(a.Err, "lor: warning: access to %s ends in %s; if you still need it, run: lor extend %s --for <duration> --reason \"...\"\n", host, formatDur(left), host)
		}
	}
	return a.execSSH(host, argv)
}

// reconcile deletes local grant directories whose grant is no longer active
// and returns the most recent such grant (if any).
func (a *App) reconcile(grants []client.Grant) *client.Grant {
	var ended *client.Grant
	for i := range grants {
		g := &grants[i]
		if g.Status == client.GrantStatusActive || g.Status == client.GrantStatusPending || !a.State.HasKey(g.Id) {
			continue
		}
		a.State.RemoveGrant(g.Id)
		if ended == nil || g.Id > ended.Id {
			ended = g
		}
	}
	if ended != nil {
		a.State.WriteSSHConfig(0, nil)
	}
	return ended
}

func nextStep(code, host string) string {
	switch code {
	case "grant_revoked":
		return "The user revoked access; don't try another way in."
	case "grant_expired", "grant_offer_expired":
		return fmt.Sprintf("Request access again if needed: lor request %s ...", host)
	case "grant_released":
		return fmt.Sprintf("Request access again if needed: lor request %s ...", host)
	}
	return ""
}

// offline uses a still-valid local certificate when Lor can't be reached;
// the command then isn't logged.
func (a *App) offline(host string, argv []string, cause error) error {
	now := a.Now()
	var best *LocalGrant
	for _, g := range a.State.LocalGrants() {
		if g.Host != host {
			continue
		}
		vb, ok := a.State.CertValidBefore(g.ID)
		if !ok || !now.Before(vb) {
			continue
		}
		g := g
		if best == nil || (g.Level == "sudo" && best.Level != "sudo") {
			best = &g
		}
	}
	if best == nil {
		return exitf(ExitSSH, "%v (and no valid local certificate for %s)", cause, host)
	}
	fmt.Fprintf(a.Err, "lor: warning: %v; using the local certificate, so this command isn't logged\n", cause)
	if err := a.State.WriteSSHConfig(best.ID, nil); err != nil {
		return exitf(ExitSSH, "%v", err)
	}
	return a.execSSH(host, argv)
}

func (a *App) execSSH(host string, argv []string) error {
	bin, err := exec.LookPath(a.SSH)
	if err != nil {
		return exitf(ExitSSH, "ssh not found: %v", err)
	}
	args := []string{bin, "-F", a.State.SSHConfigPath(), "--", "lor-" + host}
	args = append(args, argv...)
	env := slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "SSH_AUTH_SOCK=") })
	if err := a.Exec(bin, args, env); err != nil {
		return exitf(ExitSSH, "exec ssh: %v", err)
	}
	return nil
}
