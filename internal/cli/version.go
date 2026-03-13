package cli

import (
	"fmt"

	"github.com/marian2js/ocpm/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCommand(info version.Info) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print build version information",
		GroupID: "support",
		Run: func(cmd *cobra.Command, args []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", info.String())
		},
	}

	return cmd
}
