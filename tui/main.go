// Command caveira is a coding agent for abliterated models, in the terminal.
//
//	caveira                      interactive session in the current directory
//	caveira "add a --json flag"  interactive, with the first message sent
//	caveira -p "explain main.go" one-shot: run the task, print the reply, exit
//	caveira -c                   continue the last session for this directory
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/justin06lee/caveira/tui/internal/agent"
	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
	"github.com/justin06lee/caveira/tui/internal/prompt"
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

	cfg, cfgErr := config.Load(workDir)
	if model != "" {
		cfg.Model = model
	}
	if effort != "" {
		cfg.ReasoningEffort = effort
	}
	if baseURL != "" {
		cfg.BaseURL = strings.TrimRight(baseURL, "/")
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
		cfg.KeySource = "flag"
	}
	if confirm {
		cfg.Confirm = true
	}
	if cfg.ReasoningEffort != "" && !config.ValidEffort(cfg.ReasoningEffort) {
		return fmt.Errorf("reasoning effort must be one of %s", strings.Join(config.Efforts, ", "))
	}

	system := prompt.Build(prompt.Options{WorkDir: workDir, Model: cfg.Model})
	if showPrompt {
		fmt.Println(system)
		return nil
	}

	var fatal error
	if cfgErr != nil {
		fatal = cfgErr
	}
	if cfg.APIKey == "" && !isLocal(cfg.BaseURL) {
		fatal = errors.New("no API key. Put ABLITERATION_API_KEY in your environment or in a .env.local in the project, " +
			"or add \"api_key\" to " + config.Path())
	}

	client := llm.New(cfg.BaseURL, cfg.APIKey)
	ag := agent.New(client, cfg, workDir, system)

	var resumed *agent.Session
	switch {
	case resume != "":
		s, err := agent.LoadSession(resume)
		if err != nil {
			return fmt.Errorf("cannot resume %s: %w", resume, err)
		}
		ag.Attach(s)
		resumed = s
	case cont:
		s, err := agent.LatestSession(workDir)
		if err != nil {
			return err
		}
		ag.Attach(s)
		resumed = s
	}
	if ag.Session != nil {
		client.Headers["x-abliteration-session-id"] = ag.Session.ID
	} else {
		client.Headers["x-abliteration-session-id"] = ag.NewSession().ID
	}

	initial := strings.TrimSpace(strings.Join(fs.Args(), " "))

	if printMode {
		if fatal != nil {
			return fatal
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
		return runPrint(ag, initial)
	}

	m := ui.New(ui.Options{
		Agent:    ag,
		Settings: cfg,
		WorkDir:  workDir,
		Version:  version,
		Initial:  initial,
		Resumed:  resumed,
		Fatal:    fatal,
	})
	_, err = tea.NewProgram(m).Run()
	return err
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

func isLocal(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "0.0.0.0"
}

const usage = `caveira — a coding agent for abliterated models

usage
  caveira [flags] [prompt]

  With no prompt, opens an interactive session in the current directory.
  With a prompt, opens the session and sends it as the first message.

flags
  -p, --print            run the prompt non-interactively, print the reply, exit
  -m, --model <id>       model to use (default abliterated-model)
      --effort <level>   reasoning effort: none minimal low medium high xhigh max
      --base-url <url>   OpenAI-compatible endpoint (default https://api.abliteration.ai/v1)
      --api-key <key>    API key (prefer the environment or a .env.local)
  -C, --cwd <dir>        work in this directory instead of the current one
      --confirm          ask before running commands or changing files
  -c, --continue         continue the latest session for this directory
      --resume <id>      continue a specific session
      --sessions         list saved sessions
      --show-system-prompt
      --version, --help

configuration
  Environment: CAVEIRA_API_KEY or ABLITERATION_API_KEY or ABLIT_KEY,
  CAVEIRA_BASE_URL, CAVEIRA_MODEL, CAVEIRA_REASONING_EFFORT, CAVEIRA_CONFIRM.
  The same names are read from .env and .env.local files between the
  repository root and the working directory. Persistent settings live in
  ~/.caveira/config.json (api_key, base_url, model, reasoning_effort, confirm).
  Project instructions are read from CAVEIRA.md, AGENTS.md, or CLAUDE.md.
`
