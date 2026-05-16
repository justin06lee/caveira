package cli

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/justin06lee/caveira/internal/difficulty"
	"github.com/justin06lee/caveira/internal/fabricate"
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

	if cfg.Fabricate {
		return fabricatePipeline(cfg, srcPath, stagePath, srcRepo, out, errOut)
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

	for _, c := range dag.All() {
		if c.Signed {
			fmt.Fprintln(errOut, "warning: source contains GPG-signed commits; signatures will be dropped in the rewrite")
			break
		}
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
	report.WriteSummary(out, srcPath, srcPath, deadPath, before, after, span, cfg.End.Sub(cfg.Start), res.Scale, len(res.Squashes), pushed)
	return 0
}

// fabricatePipeline runs the --fabricate flow: resolve identities, generate a
// synthetic Plan, schedule it, and write it to the staged repo.
func fabricatePipeline(cfg *input.Config, srcPath, stagePath string, srcRepo *git.Repository, out, errOut io.Writer) int {
	mode := "single"
	nIDs := 1
	var rawIDs []string
	switch {
	case cfg.PigsN > 0:
		mode = "pigs"
		nIDs = cfg.PigsN
		rawIDs = cfg.PigIdentities
	case cfg.RatsN > 0:
		mode = "rats"
		nIDs = cfg.RatsN
		rawIDs = cfg.RatIdentities
	}

	mailmap, err := fabricate.LoadMailmap(srcPath)
	if err != nil {
		fmt.Fprintln(errOut, "error: read .mailmap:", err)
		return 1
	}

	var ids []fabricate.Identity
	if mode == "single" {
		id, err := singleAuthorIdentity()
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}
		ids = []fabricate.Identity{id}
	} else {
		resolved, err := fabricate.ResolveIdentities(srcRepo, rawIDs, nIDs, mailmap, cfg.Pick, os.Stdin, out)
		if err != nil {
			fmt.Fprintln(errOut, "error: identity resolution:", err)
			return 1
		}
		ids = resolved
	}

	modelReport, err := fabricate.ScanModelReport(srcRepo, mailmap)
	if err != nil {
		fmt.Fprintln(errOut, "error: scan models:", err)
		return 1
	}

	rng := rngFor(cfg)
	var weights []int
	if cfg.Earned {
		discovered, derr := fabricate.DiscoverIdentities(srcRepo, mailmap)
		if derr != nil {
			fmt.Fprintln(errOut, "error: discover identities for --earned:", derr)
			return 1
		}
		weights = fabricate.EarnedWeights(ids, discovered, mailmap)
		if weights == nil {
			fmt.Fprintln(errOut, "note: --earned: no identities found in history; using equal weights")
		}
	}
	plan, dag, err := fabricate.Generate(srcRepo, ids, weights, mode, rng)
	if err != nil {
		fmt.Fprintln(errOut, "error: fabricate generate:", err)
		return 1
	}
	if len(dag.All()) == 0 {
		fmt.Fprintln(errOut, "error: fabricator produced no commits")
		return 1
	}

	origCommitCount := len(plan.Commits)

	durations, diffs := schedule.BuildDurations(dag, rng)
	res, err := schedule.Schedule(dag, durations, cfg.Start, cfg.End)
	if err != nil {
		fmt.Fprintln(errOut, "error:", err)
		return 1
	}

	// When the window is too narrow the scheduler squashes linear edges to
	// make the history fit. Pigs histories are purely linear, so we can
	// faithfully replay those squashes onto the Plan and write the squashed
	// (surviving) history. Single and rats modes refuse instead, since
	// squashing fabricated commits there defeats the purpose.
	if len(res.Squashes) > 0 {
		if mode == "pigs" {
			ops := make([]fabricate.SquashOp, len(res.Squashes))
			for i, sq := range res.Squashes {
				ops[i] = fabricate.SquashOp{Parent: sq.Parent, Child: sq.Child}
			}
			if err := fabricate.SquashPlan(plan, ops, durations); err != nil {
				fmt.Fprintln(errOut, "error: fabricate squash:", err)
				return 1
			}
		} else {
			fmt.Fprintf(errOut, "error: the time window is too small for %d fabricated commits; widen --start/--end (or use --pigs, which squashes to fit)\n", origCommitCount)
			return 1
		}
	}

	if cfg.DryRun {
		rows := rowsFor(dag, durations, diffs, res, srcRepo)
		report.WriteDryRun(out, rows, res, cfg.Start, cfg.End)
		return 0
	}

	fabricate.InjectCoAuthors(plan, modelReport, rng)

	if _, err := os.Stat(stagePath); err == nil {
		fmt.Fprintf(errOut, "error: %s already exists; remove or rename before retrying\n", stagePath)
		return 1
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

	if _, err := fabricate.WriteToRepo(srcRepo, stageRepo, plan, res.NewTimes); err != nil {
		fmt.Fprintln(errOut, "error: fabricate write:", err)
		return 1
	}

	if err := verifyTreeMatch(srcRepo, stageRepo); err != nil {
		fmt.Fprintln(errOut, "error: fabricated tree does not match source:", err)
		return 1
	}

	if err := resetWorktreeToHead(stagePath); err != nil {
		fmt.Fprintln(errOut, "warn: reset worktree:", err)
	}
	_ = exec.Command("git", "-C", stagePath, "reflog", "expire", "--expire=now", "--all").Run()
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

	before := origCommitCount
	after := len(plan.Commits)
	span := windowSpan(res, cfg.Start)
	report.WriteSummary(out, srcPath, srcPath, deadPath, before, after, span, cfg.End.Sub(cfg.Start), res.Scale, len(res.Squashes), pushed)
	return 0
}

// verifyTreeMatch confirms the rewritten repo's HEAD tree equals the source
// repo's HEAD tree — a defensive backstop for fabrication correctness.
func verifyTreeMatch(src, dst *git.Repository) error {
	srcTree, err := headTreeHash(src)
	if err != nil {
		return fmt.Errorf("source head tree: %w", err)
	}
	dstTree, err := headTreeHash(dst)
	if err != nil {
		return fmt.Errorf("rewritten head tree: %w", err)
	}
	if srcTree != dstTree {
		return fmt.Errorf("tree %s != %s", dstTree, srcTree)
	}
	return nil
}

func headTreeHash(r *git.Repository) (plumbing.Hash, error) {
	head, err := r.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return commit.TreeHash, nil
}

// singleAuthorIdentity reads git config user.{name,email} via the system git
// binary (Caveira already shells out for `git gc`).
func singleAuthorIdentity() (fabricate.Identity, error) {
	name, err := exec.Command("git", "config", "user.name").CombinedOutput()
	if err != nil {
		return fabricate.Identity{}, fmt.Errorf("git config user.name: %w", err)
	}
	email, err := exec.Command("git", "config", "user.email").CombinedOutput()
	if err != nil {
		return fabricate.Identity{}, fmt.Errorf("git config user.email: %w", err)
	}
	n := strings.TrimSpace(string(name))
	e := strings.TrimSpace(string(email))
	if n == "" || e == "" {
		return fabricate.Identity{}, fmt.Errorf("git config user.{name,email} not set; pass --pig \"Name <email>\"")
	}
	return fabricate.Identity{Name: n, Email: e}, nil
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
			NewTime:    res.NewTimes[oid].In(c.AuthorDate.Location()),
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
