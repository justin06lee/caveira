// Command caveira is a coding agent for abliterated models, in the terminal.
//
//	caveira                      interactive session in the current directory
//	caveira "add a --json flag"  interactive, with the first message sent
//	caveira -p "explain main.go" one-shot: run the task, print the reply, exit
//	caveira -c                   continue the last session for this directory
//	caveira login                sign in to a caveira account, without the TUI
//	caveira logout               sign out of it
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/cloud"
	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/prompt"
	"github.com/justin06lee/caveira/tui/internal/ui"
)

// version is set by the Makefile from the git describe output.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "caveira:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("caveira", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		printMode  bool
		model      string
		effort     string
		baseURL    string
		apiKey     string
		cwd        string
		confirm    bool
		cont       bool
		resume     string
		showVer    bool
		showHelp   bool
		showPrompt bool
		listSess   bool
	)
	fs.BoolVar(&printMode, "p", false, "")
	fs.BoolVar(&printMode, "print", false, "")
	fs.StringVar(&model, "m", "", "")
	fs.StringVar(&model, "model", "", "")
	fs.StringVar(&effort, "effort", "", "")
	fs.StringVar(&baseURL, "base-url", "", "")
	fs.StringVar(&apiKey, "api-key", "", "")
	fs.StringVar(&cwd, "C", "", "")
	fs.StringVar(&cwd, "cwd", "", "")
	fs.BoolVar(&confirm, "confirm", false, "")
	fs.BoolVar(&cont, "c", false, "")
	fs.BoolVar(&cont, "continue", false, "")
	fs.StringVar(&resume, "resume", "", "")
	fs.BoolVar(&showVer, "version", false, "")
	fs.BoolVar(&showHelp, "h", false, "")
	fs.BoolVar(&showHelp, "help", false, "")
	fs.BoolVar(&showPrompt, "show-system-prompt", false, "")
	fs.BoolVar(&listSess, "sessions", false, "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "caveira:", err)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if showHelp {
		fmt.Print(usage)
		return nil
	}
	if showVer {
		fmt.Println("caveira", version)
		return nil
	}
	// login and logout alone on the line are commands; with more words
	// after them they are a prompt like any other.
	if fs.NArg() == 1 {
		switch fs.Arg(0) {
		case "login":
			return login(os.Stdout, cloud.OpenBrowser)
		case "logout":
			return logout(os.Stdout)
		}
	}

	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	if cwd != "" {
		workDir, err = filepath.Abs(cwd)
		if err != nil {
			return err
		}
		if st, err := os.Stat(workDir); err != nil || !st.IsDir() {
			return fmt.Errorf("%s is not a directory", workDir)
		}
	}

	if listSess {
		sessions, err := agent.ListSessions("")
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no saved sessions")
			return nil
		}
		for _, s := range sessions {
			fmt.Printf("%s  %s  %s\n  %s\n", s.ID, s.UpdatedAt.Format("2006-01-02 15:04"), s.WorkDir, s.Title)
		}
		return nil
	}

	initial := strings.TrimSpace(strings.Join(fs.Args(), " "))

	if showPrompt {
		fmt.Println(prompt.Build(prompt.Options{WorkDir: workDir, Model: firstNonEmpty(model, config.DefaultModel)}))
		return nil
	}

	// settings resolves the configuration with the command line laid over
	// it. The session reads it again after /login and /logout.
	settings := func() (config.Settings, error) {
		cfg, err := config.Load(workDir)
		// abliteration.ai is reached through the account, so a key or an
		// endpoint for it on the command line changes nothing. Another
		// endpoint is used as given, and the account's token only ever
		// goes to the caveira site.
		if cfg.OnAccount() && baseURL != "" && strings.TrimRight(baseURL, "/") != config.DefaultBaseURL {
			cfg = cfg.WithoutAccount()
		}
		if model != "" {
			cfg.Model = model
		}
		if baseURL != "" && !cfg.OnAccount() {
			cfg.BaseURL = strings.TrimRight(baseURL, "/")
		}
		if apiKey != "" && !cfg.OnAccount() {
			cfg.APIKey = apiKey
			cfg.KeySource = "flag"
		}
		if effort != "" {
			cfg.ReasoningEffort = effort
		}
		if confirm {
			cfg.Confirm = true
		}
		return cfg, err
	}

	// load does the startup work: settings, system prompt, client, agent,
	// and the session to continue. The TUI runs it behind its intro.
	// Signed out, the TUI still starts, and says to /login.
	load := func() (ui.Options, error) {
		cfg, cfgErr := settings()
		if cfg.ReasoningEffort != "" && !config.ValidEffort(cfg.ReasoningEffort) {
			return ui.Options{}, fmt.Errorf("reasoning effort must be one of %s", strings.Join(config.Efforts, ", "))
		}

		system := prompt.Build(prompt.Options{WorkDir: workDir, Model: cfg.Model})

		client := llm.New(cfg.BaseURL, cfg.APIKey)
		ag := agent.New(client, cfg, workDir, system)

		var resumed *agent.Session
		switch {
		case resume != "":
			s, err := agent.LoadSession(resume)
			if err != nil {
				return ui.Options{}, fmt.Errorf("cannot resume %s: %w", resume, err)
			}
			ag.Attach(s)
			resumed = s
		case cont:
			s, err := agent.LatestSession(workDir)
			if err != nil {
				return ui.Options{}, err
			}
			ag.Attach(s)
			resumed = s
		}
		if ag.Session != nil {
			client.Headers["x-abliteration-session-id"] = ag.Session.ID
		} else {
			client.Headers["x-abliteration-session-id"] = ag.NewSession().ID
		}
		return ui.Options{
			Agent:    ag,
			Settings: cfg,
			WorkDir:  workDir,
			Branch:   gitBranch(workDir),
			Version:  version,
			Initial:  initial,
			Resumed:  resumed,
			Fatal:    cfgErr,
			Reload:   settings,
		}, nil
	}

	if printMode {
		o, err := load()
		if err != nil {
			return err
		}
		if o.Fatal != nil {
			return o.Fatal
		}
		if ui.KeyMissing(o.Settings) {
			return errors.New(`not signed in. Run "caveira login" to sign in to your caveira account, then try again`)
		}
		if initial == "" || initial == "-" {
			b, err := io.ReadAll(bufio.NewReader(os.Stdin))
			if err != nil {
				return err
			}
			initial = strings.TrimSpace(string(b))
		}
		if initial == "" {
			return errors.New("nothing to do: pass a prompt as an argument or on stdin")
		}
		return runPrint(o.Agent, initial)
	}

	return ui.Run(version, load)
}

// runPrint is the non-interactive path: the reply streams to stdout, tool
// activity goes to stderr so the two can be separated.
func runPrint(ag *agent.Agent, input string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	var failed error
	wroteText := false

	ag.Run(ctx, input, func(ev agent.Event) {
		switch ev := ev.(type) {
		case agent.TextEvent:
			delta := ev.Delta
			if !wroteText {
				// Models often open with a blank line or two; the shell
				// prompt above is separation enough.
				delta = strings.TrimLeft(delta, "\n")
				if delta == "" {
					return
				}
			}
			out.WriteString(delta)
			out.Flush()
			wroteText = true
		case agent.AssistantDoneEvent:
			if wroteText {
				out.WriteString("\n")
				out.Flush()
				wroteText = false
			}
		case agent.ToolStartEvent:
			fmt.Fprintf(os.Stderr, "⚙ %s  %s\n", ev.Name, ev.Preview)
		case agent.ToolEndEvent:
			mark := "  ↳"
			if ev.Result.IsError {
				mark = "  ✗"
			}
			fmt.Fprintf(os.Stderr, "%s %s\n", mark, ev.Result.Summary)
		case agent.ApprovalEvent:
			// No one to ask in print mode; --confirm applies to the TUI.
			ev.Reply <- agent.Allow
		case agent.CompactEvent:
			fmt.Fprintf(os.Stderr, "── context compacted (%d messages) ──\n", ev.BeforeMessages)
		case agent.ErrorEvent:
			failed = ev.Err
			fmt.Fprintln(os.Stderr, "✗", ev.Err)
		case agent.DoneEvent:
			if ev.Interrupted {
				failed = errors.New("interrupted")
			}
		}
	})
	t := ag.Totals
	fmt.Fprintf(os.Stderr, "── %d requests · %d in · %d out · $%.4f ──\n", t.Requests, t.InputTokens, t.OutputTokens, t.CostUSD)
	return failed
}

// gitBranch is the branch checked out in dir, or "" outside a repository.
func gitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// login is /login without the TUI, for a machine reached over SSH or one
// with no screen: it prints the page and the code, opens the browser if it
// can, and waits until the page has been approved.
func login(out io.Writer, open func(string) error) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if a, ok := cloud.Load(); ok {
		// A token the site no longer takes is replaced; any other is kept.
		check, done := context.WithTimeout(ctx, 15*time.Second)
		_, err := cloud.Me(check, a)
		done()
		if !cloud.Unauthorized(err) {
			fmt.Fprintf(out, "Already signed in as %s. Run \"caveira logout\" first to use another account.\n", a.Email)
			return nil
		}
	}

	site := cloud.URL()
	code, err := cloud.Start(ctx, site, "tui", cloud.Label())
	if err != nil {
		return fmt.Errorf("cannot start the login: %w", err)
	}
	lead := "Opening your browser to sign in to caveira. If it didn't open, visit"
	if open(code.VerifyURL) != nil {
		lead = "To sign in to caveira, visit"
	}
	fmt.Fprintf(out, "%s\n\n  %s\n\nand check the page shows the code %s\n\nWaiting for the browser… (ctrl+c cancels)\n",
		lead, code.VerifyURL, code.UserCode)

	a, err := cloud.Wait(ctx, site, code)
	if ctx.Err() != nil {
		return errors.New("login cancelled")
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Signed in as", a.Email)
	return nil
}

// logout ends the account's session on the caveira site and forgets it
// on this machine.
func logout(out io.Writer) error {
	a, ok := cloud.Load()
	if !ok {
		fmt.Fprintln(out, "Not signed in.")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := cloud.Logout(ctx)
	if _, still := cloud.Load(); still {
		return fmt.Errorf("cannot sign out: %w", err)
	}
	fmt.Fprintln(out, "Signed out of", a.Email)
	if err != nil {
		// The token is gone from here; the site may still take it.
		return fmt.Errorf("the caveira site could not be told: %w", err)
	}
	return nil
}

const usage = `caveira — a coding agent for abliterated models

usage
  caveira [flags] [prompt]      (or cav, the same program)
  caveira login                 sign in to your caveira account
  caveira logout                sign out

  With no prompt, opens an interactive session in the current directory.
  With a prompt, opens the session and sends it as the first message.

flags
  -p, --print            run the prompt non-interactively, print the reply, exit
  -m, --model <id>       model to use (default abliterated-model)
      --effort <level>   reasoning effort: none minimal low medium high xhigh max
      --base-url <url>   another OpenAI-compatible server, for development
                         (default: your caveira account)
      --api-key <key>    the key for that server (prefer the environment)
  -C, --cwd <dir>        work in this directory instead of the current one
      --confirm          ask before running commands or changing files
  -c, --continue         continue the latest session for this directory
      --resume <id>      continue a specific session
      --sessions         list saved sessions
      --show-system-prompt
      --version, --help

signing in
  caveira login (or /login in a session) opens caveira in the browser; once
  you connect it there, model requests go through your caveira account. It
  prints the page and the code too, for a machine reached over SSH. Without
  an account caveira does not send messages; a key of your own does not
  stand in for one. For development, --base-url points it at another server.
  CAVEIRA_CLOUD_URL points sign-in at another caveira site.

configuration
  Environment: CAVEIRA_API_KEY or ABLITERATION_API_KEY or ABLIT_KEY,
  CAVEIRA_BASE_URL, CAVEIRA_MODEL, CAVEIRA_REASONING_EFFORT, CAVEIRA_CONFIRM.
  The same names are read from .env and .env.local files between the
  repository root and the working directory. Persistent settings live in
  ~/.caveira/config.json (api_key, base_url, model, reasoning_effort, confirm).
  Project instructions are read from CAVEIRA.md, AGENTS.md, or CLAUDE.md.
`

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
