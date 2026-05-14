package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Run executes the CLI using os.Args and the standard streams.
// Returns the process exit code.
func Run() int {
	return RunWithArgs(os.Args[1:], os.Stdout, os.Stderr)
}

// RunWithArgs executes the CLI with the given args and output streams.
// Returns the process exit code. Used by tests and by Run().
func RunWithArgs(args []string, stdout, stderr io.Writer) int {
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "caveira",
		Short: "Rewrite a repo's commit timestamps to fit a chosen time window",
		Long: `caveira duplicates a git repository and rewrites the commit history
in the copy so that every commit falls inside a user-supplied time window,
with per-commit durations inferred from each commit's difficulty.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
