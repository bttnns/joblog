package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the build version, overridable at link time with
// -ldflags "-X github.com/bttnns/joblog/internal/cli.Version=v1.2.3".
var Version = "dev"

func init() { addCommand(newVersionCmd) }

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the jl version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantJSON(cmd) {
				return emitJSON(map[string]string{"version": Version})
			}
			fmt.Println(Version)
			return nil
		},
	}
}
