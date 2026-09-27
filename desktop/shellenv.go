package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// An app opened from the Finder or the Dock inherits launchd's environment:
// PATH is /usr/bin:/bin:/usr/sbin:/sbin and nothing from the user's shell
// profile is there. The agent's bash tool would not find go, bun, node, or
// anything else installed through Homebrew, and an API key exported in
// .zshrc would not be seen. So the app asks the user's login shell for its
// environment once at startup, the way editors do, and takes PATH and
// anything it does not already have from it.

const envMarker = "__CAVEIRA_ENV__"

var (
	envOnce sync.Once
	envDone = make(chan struct{})
)

// loadShellEnv runs once; waitShellEnv blocks until it has.
func loadShellEnv() {
	envOnce.Do(func() {
		defer close(envDone)
		if os.Getenv("TERM") != "" {
			// Started from a terminal: the environment is already the shell's.
			return
		}
		applyEnv(shellEnv(3 * time.Second))
	})
}

func waitShellEnv() {
	go loadShellEnv()
	select {
	case <-envDone:
	case <-time.After(4 * time.Second):
	}
}

// shellEnv is the login shell's environment, or nil if it cannot be had in
// time. The marker skips whatever the profile prints on the way in.
func shellEnv(timeout time.Duration) map[string]string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc", "printf '"+envMarker+"'; command env -0")
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "CAVEIRA_RESOLVING_ENV=1")
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseEnv(out)
}

func parseEnv(out []byte) map[string]string {
	i := bytes.LastIndex(out, []byte(envMarker))
	if i < 0 {
		return nil
	}
	env := map[string]string{}
	for _, kv := range bytes.Split(out[i+len(envMarker):], []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if !ok || k == "" || strings.ContainsAny(k, "\n ") {
			continue
		}
		env[k] = v
	}
	return env
}

// skipEnv are shell bookkeeping, not environment.
var skipEnv = map[string]bool{"PWD": true, "OLDPWD": true, "SHLVL": true, "_": true, "CAVEIRA_RESOLVING_ENV": true}

func applyEnv(env map[string]string) {
	for k, v := range env {
		if skipEnv[k] {
			continue
		}
		if _, set := os.LookupEnv(k); set && k != "PATH" {
			continue
		}
		os.Setenv(k, v)
	}
}
