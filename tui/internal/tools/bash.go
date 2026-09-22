package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	bashDefaultTimeout = 2 * time.Minute
	bashMaxTimeout     = 10 * time.Minute
	bashOutputLimit    = 30_000
)

type bashTool struct{ dir string }

func (t *bashTool) Name() string { return "bash" }
func (t *bashTool) Kind() Kind   { return KindExecute }
func (t *bashTool) Description() string {
	return "Run a shell command in the working directory and return its combined stdout and stderr plus the exit code. " +
		"Use it for builds, tests, git, package managers, and anything else with a CLI. The shell is non-interactive: " +
		"commands that prompt for input will hang until the timeout, so pass flags like -y or --no-edit. " +
		"Prefer the dedicated read_file, edit_file, glob, and grep tools over cat, sed, find, and grep. " +
		"Do not use cd to move around; give absolute paths or paths relative to the working directory instead. " +
		"Long output is truncated in the middle; pipe through head, tail, or grep when you only need part of it."
}
func (t *bashTool) Schema() map[string]any {
	return schema(map[string]any{
		"command":     prop("string", "The command to run, as you would type it in bash."),
		"description": prop("string", "A short description of what the command does, in plain words, shown to the user."),
		"timeout":     prop("integer", "Timeout in seconds. Defaults to 120, maximum 600."),
	}, "command")
}
func (t *bashTool) Preview(args json.RawMessage) string {
	var a struct {
		Command string `json:"command"`
	}
	_ = decode(args, &a)
	return strings.TrimSpace(a.Command)
}

func (t *bashTool) Run(ctx context.Context, args json.RawMessage) Result {
	var a struct {
		Command     string `json:"command"`
		Description string `json:"description"`
		Timeout     int    `json:"timeout"`
	}
	if err := decode(args, &a); err != nil {
		return errorResult("bad arguments: %v", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return errorResult("command is required")
	}
	timeout := bashDefaultTimeout
	if a.Timeout > 0 {
		timeout = time.Duration(a.Timeout) * time.Second
		// Some models send milliseconds out of habit.
		if a.Timeout > 600 && a.Timeout%1000 == 0 {
			timeout = time.Duration(a.Timeout) * time.Millisecond
		}
	}
	if timeout > bashMaxTimeout {
		timeout = bashMaxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := "bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", a.Command)
	cmd.Dir = t.dir
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		"CAVEIRA=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"PAGER=cat",
		"TERM=dumb",
		"NO_COLOR=1",
	)
	// Own process group, so killing on timeout takes the children with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start).Round(10 * time.Millisecond)

	exit := 0
	var status string
	switch {
	case err == nil:
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		exit = -1
		status = fmt.Sprintf("Command timed out after %s and was killed.", timeout)
	case errors.Is(ctx.Err(), context.Canceled):
		exit = -1
		status = "Command was cancelled."
	default:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
			if exit == -1 && ee.ProcessState != nil {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					status = fmt.Sprintf("Command was killed by signal %s.", ws.Signal())
				}
			}
		} else {
			return errorResult("could not start command: %v", err)
		}
	}

	text := strings.TrimRight(out.String(), "\n")
	text, truncated := truncateOutput(text, bashOutputLimit)

	var sb strings.Builder
	if text != "" {
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	if status != "" {
		sb.WriteString(status)
		sb.WriteString("\n")
	}
	if exit != 0 {
		fmt.Fprintf(&sb, "[exit code %d]", exit)
	} else if text == "" {
		sb.WriteString("[no output, exit code 0]")
	}
	if truncated {
		sb.WriteString("\n[output was truncated; rerun with head/tail/grep to see a specific part]")
	}

	lines := 0
	if text != "" {
		lines = strings.Count(text, "\n") + 1
	}
	summary := fmt.Sprintf("exit %d · %s · %s", exit, plural(lines, "line"), elapsed)
	if status != "" {
		summary = strings.TrimSuffix(status, ".")
	}
	return Result{Output: sb.String(), Summary: summary, IsError: exit != 0}
}
