package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/justin06lee/caveira/internal/input"
	"github.com/spf13/cobra"
)

// errPipelineFailed is returned by RunE when Pipeline has already reported the
// failure to stderr and exited nonzero. RunWithArgs recognizes it and avoids
// printing a second, redundant error line.
var errPipelineFailed = errors.New("pipeline failed")

func Run() int {
	return RunWithArgs(os.Args[1:], os.Stdout, os.Stderr)
}

func RunWithArgs(args []string, stdout, stderr io.Writer) int {
	cmd := newRootCmd(invocationName())
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		// Pipeline failures are already reported on stderr; only print errors
		// that originate here (flag parsing, validation, datetime parsing).
		if !errors.Is(err, errPipelineFailed) {
			fmt.Fprintln(stderr, "error:", err)
		}
		return 1
	}
	return 0
}

func invocationName() string {
	if len(os.Args) == 0 {
		return "caveira"
	}
	name := filepath.Base(os.Args[0])
	if name == "" || name == "." || name == "/" {
		return "caveira"
	}
	return name
}

func newRootCmd(name string) *cobra.Command {
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
		preserve  bool

		fabricateFlag bool
		pigsN         int
		ratsN         int
		pigIDs        []string
		ratIDs        []string
		pickFlag      bool
		earnedFlag    bool

		leechesN int
		leechIDs []string
	)

	cmd := &cobra.Command{
		Use:           name,
		Short:         "Rewrite a repo's commit timestamps to fit a chosen time window",
		SilenceErrors: true,
		Example: "  " + name + ` --repo /path/to/myrepo \
      --start "2026-05-14 13:00" \
      --end   "2026-05-14 17:00"

  ` + name + ` --repo /path/to/myrepo --preserve \
      --start "2026-05-14 13:00" --end "2026-05-14 13:30"

  ` + name + ` --repo /path/to/myrepo --fabricate \
      --start "2026-05-14 09:00" --end "2026-05-14 17:00"

  ` + name + ` --repo /path/to/myrepo --fabricate \
      --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
      --pigs 3

  ` + name + ` --repo /path/to/myrepo \
      --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
      --leeches 3 --leech "Alice <a@x.com>" --leech "Bob <b@x.com>"`,
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
				Preserve:      preserve,
				Fabricate:     fabricateFlag,
				PigsN:         pigsN,
				RatsN:         ratsN,
				PigIdentities: pigIDs,
				RatIdentities: ratIDs,
				Pick:          pickFlag,
				Earned:        earnedFlag,

				LeechesN:        leechesN,
				LeechIdentities: leechIDs,
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			code := Pipeline(cfg, c.OutOrStdout(), c.ErrOrStderr())
			if code != 0 {
				// Pipeline already printed the cause; signal failure without a
				// duplicate error line (see errPipelineFailed).
				return errPipelineFailed
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
	cmd.Flags().BoolVar(&preserve, "preserve", false, "never merge commits; keep all of them in their original chronological order, compressing the spacing to fit the window")

	cmd.Flags().BoolVar(&fabricateFlag, "fabricate", false, "synthesize a new commit history instead of retiming the source")
	cmd.Flags().IntVar(&pigsN, "pigs", 0, "chaotic single-branch fabricator with N people (requires --fabricate)")
	cmd.Flags().IntVar(&ratsN, "rats", 0, "branched fabricator with N people (requires --fabricate)")
	cmd.Flags().StringArrayVar(&pigIDs, "pig", nil, "pig identity as \"Name <email>\"; repeatable (requires --pigs)")
	cmd.Flags().StringArrayVar(&ratIDs, "rat", nil, "rat identity as \"Name <email>\"; repeatable (requires --rats)")
	cmd.Flags().BoolVar(&pickFlag, "pick", false, "always open the interactive player picker (requires --pigs/--rats/--leeches)")
	cmd.Flags().BoolVar(&earnedFlag, "earned", false, "weight author assignment by real commit-count distribution (requires --pigs/--rats)")

	cmd.Flags().IntVar(&leechesN, "leeches", 0, "retime the existing history and scatter authorship across N people plus the original authors")
	cmd.Flags().StringArrayVar(&leechIDs, "leech", nil, "leech identity as \"Name <email>\"; repeatable (requires --leeches)")

	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")

	cmd.AddCommand(newInterrogateCmd())

	return cmd
}
