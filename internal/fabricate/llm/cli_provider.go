package llm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// cliProvider runs an installed CLI as a subprocess, feeding the prompt on
// stdin and capturing stdout as the raw response.
type cliProvider struct {
	name   string
	binary string
	args   []string
	opts   Options
}

func newCLIProvider(name, binary string, args []string, opts Options) *cliProvider {
	full := append([]string{}, args...)
	if opts.Model != "" {
		full = append(full, "--model", opts.Model)
	}
	return &cliProvider{name: name, binary: binary, args: full, opts: opts}
}

func (p *cliProvider) Name() string { return p.name }

func (p *cliProvider) GeneratePlan(ctx context.Context, prompt string) (string, error) {
	if _, err := exec.LookPath(p.binary); err != nil {
		return "", fmt.Errorf("%s: %q not found on PATH; install it or choose another engine", p.name, p.binary)
	}
	ctx, cancel := context.WithTimeout(ctx, p.opts.timeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, p.binary, p.args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %q failed: %w: %s", p.name, p.binary, err,
			truncate(strings.TrimSpace(stderr.String()), 300))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("%s: %q produced no output", p.name, p.binary)
	}
	return out, nil
}
