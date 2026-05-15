package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/justin06lee/caveira/internal/input"
	"github.com/spf13/cobra"
)

func Run() int {
	return RunWithArgs(os.Args[1:], os.Stdout, os.Stderr)
}

func RunWithArgs(args []string, stdout, stderr io.Writer) int {
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	var (
		repoFlag  string
		startFlag string
		endFlag   string
		seedFlag  int64
		dryRun    bool
		pushFlag  bool
		pushProt  bool
		windowTZ  string
		outDir    string
	)

	cmd := &cobra.Command{
		Use:          "caveira",
		Short:        "Rewrite a repo's commit timestamps to fit a chosen time window",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			tz, err := time.LoadLocation(windowTZ)
			if err != nil {
				return fmt.Errorf("invalid --window-tz %q: %w", windowTZ, err)
			}
			now := time.Now().In(tz)
			start, err := input.ParseDateTime(startFlag, tz, now)
			if err != nil {
				return err
			}
			end, err := input.ParseDateTime(endFlag, tz, now)
			if err != nil {
				return err
			}
			cfg := &input.Config{
				Repo:          repoFlag,
				Start:         start,
				End:           end,
				Seed:          seedFlag,
				HasSeed:       c.Flags().Changed("seed"),
				DryRun:        dryRun,
				Push:          pushFlag,
				PushProtected: pushProt,
				WindowTZ:      tz,
				OutDir:        outDir,
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			code := Pipeline(cfg, c.OutOrStdout(), c.ErrOrStderr())
			if code != 0 {
				return fmt.Errorf("caveira exited with code %d", code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "path or URL of the source repository (required)")
	cmd.Flags().StringVar(&startFlag, "start", "", "window start (required)")
	cmd.Flags().StringVar(&endFlag, "end", "", "window end (required)")
	cmd.Flags().Int64Var(&seedFlag, "seed", 0, "deterministic seed for duration draws")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the schedule, write nothing")
	cmd.Flags().BoolVar(&pushFlag, "push", false, "force-push to origin after the swap")
	cmd.Flags().BoolVar(&pushProt, "push-protected", false, "allow pushing main/master")
	cmd.Flags().StringVar(&windowTZ, "window-tz", "Local", "IANA timezone for --start/--end")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "parent directory for URL clones (default $CWD)")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")

	return cmd
}
