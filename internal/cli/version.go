package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/stackradar/stackradar-cli/internal/buildinfo"
)

func newVersionCommand(streams Streams) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(command *cobra.Command, args []string) error {
			info := buildinfo.Current()
			_, err := fmt.Fprintf(streams.Out, "stackradar %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)

			return err
		},
	}
}
