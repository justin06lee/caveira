package cli

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/input"
	"github.com/justin06lee/caveira/internal/repo"
	"github.com/justin06lee/caveira/internal/report"
	"github.com/justin06lee/caveira/internal/rewrite"
	"github.com/justin06lee/caveira/internal/schedule"
	"github.com/justin06lee/caveira/internal/walk"
)

// Pipeline runs the full caveira flow per cfg, writing user-facing output
// to out and errors to errOut. Returns a process exit code.
func Pipeline(cfg *input.Config, out, errOut io.Writer) int {
	srcPath, _, err := acquireSource(cfg)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}

	stagePath := srcPath + ".interrogating"

	if _, err := os.Stat(stagePath); err == nil {
		fmt.Fprintf(errOut, "error: %s already exists; remove or rename before retrying\n", stagePath)
		return 1
	}

	srcRepo, err := git.PlainOpen(srcPath)
	if err != nil {
		fmt.Fprintln(errOut, "error: opening source:", err)
		return 1
	}
	dag, err := walk.Load(srcRepo)
	if err != nil {
		fmt.Fprintln(errOut, "error: loading DAG:", err)
		return 1
	}
	if len(dag.All()) == 0 {
		fmt.Fprintln(errOut, "error: source has no commits")
		return 1
	}

	rng := rngFor(cfg)
	durations, diffs := schedule.BuildDurations(dag, rng)

	res, err := schedule.Schedule(dag, durations, cfg.Start, cfg.End)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}

	if cfg.DryRun {
		rows := rowsFor(dag, durations, diffs, res, srcRepo)
		report.WriteDryRun(out, rows, res, cfg.Start, cfg.End)
		return 0
	}

	if err := repo.Duplicate(srcPath, stagePath); err != nil {
		fmt.Fprintln(errOut, "error: duplicate:", err)
		return 1
	}

	stageRepo, err := git.PlainOpen(stagePath)
	if err != nil {
		fmt.Fprintln(errOut, "error: open staged repo:", err)
		return 1
	}

	mapping, err := rewrite.Apply(srcRepo, stageRepo, dag, res)
	if err != nil {
		fmt.Fprintln(errOut, "error: rewrite apply:", err)
		return 1
	}
	if err := rewrite.RebuildRefs(srcRepo, stageRepo, mapping); err != nil {
		fmt.Fprintln(errOut, "error: rebuild refs:", err)
		return 1
	}
	if err := resetWorktreeToHead(stagePath); err != nil {
		fmt.Fprintln(errOut, "warn: reset worktree:", err)
	}
	_ = exec.Command("git", "-C", stagePath, "gc", "--prune=now").Run()

	deadPath, err := repo.Swap(srcPath, stagePath)
	if err != nil {
		fmt.Fprintln(errOut, "error: swap:", err)
		return 1
	}

	pushed := false
	if cfg.Push {
		if err := repo.Push(srcPath, cfg.PushProtected); err != nil {
			fmt.Fprintln(errOut, "error: push:", err)
			return 1
		}
		pushed = true
	}

	before := len(dag.All())
	after := before - len(res.Squashes)
	span := windowSpan(res, cfg.Start)
	report.WriteSummary(out, deadPath, srcPath, deadPath, before, after, span, cfg.End.Sub(cfg.Start), res.Scale, len(res.Squashes), pushed)
	return 0
}

func acquireSource(cfg *input.Config) (path, name string, err error) {
	if repo.IsURL(cfg.Repo) {
		dst, err := repo.CloneURL(cfg.Repo, cfg.OutDir)
		if err != nil {
			return "", "", err
		}
		return dst, filepath.Base(dst), nil
	}
	abs, err := filepath.Abs(cfg.Repo)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("source path %s is not a directory", abs)
	}
	return abs, filepath.Base(abs), nil
}

func rngFor(cfg *input.Config) *rand.Rand {
	if cfg.HasSeed {
		return rand.New(rand.NewSource(cfg.Seed))
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

func rowsFor(dag *walk.DAG, durations map[string]int, diffs map[string]difficulty.Difficulty, res *schedule.Result, src *git.Repository) []report.Row {
	var rows []report.Row
	order, _ := dag.TopologicalOrder()
	for _, oid := range order {
		c := dag.Get(oid)
		row := report.Row{
			ShortOID:   oid[:7],
			Difficulty: diffs[oid],
			Duration:   durations[oid],
			OldTime:    c.AuthorDate,
			NewTime:    res.NewTimes[oid],
		}
		rows = append(rows, row)
	}
	return rows
}

func windowSpan(res *schedule.Result, start time.Time) time.Duration {
	var maxT time.Time
	for _, t := range res.NewTimes {
		if t.After(maxT) {
			maxT = t
		}
	}
	return maxT.Sub(start)
}

func resetWorktreeToHead(path string) error {
	r, err := git.PlainOpen(path)
	if err != nil {
		return err
	}
	wt, err := r.Worktree()
	if err != nil {
		return err
	}
	head, err := r.Head()
	if err != nil {
		return err
	}
	return wt.Reset(&git.ResetOptions{Commit: head.Hash(), Mode: git.HardReset})
}
