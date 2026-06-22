package cli

import (
	"github.com/spf13/cobra"
)

func init() { addCommand(newInitCmd) }

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold the data directory (you do not normally need to run this)",
		Long: "Create the data directory tree wherever the resolver lands (see --data-dir\n" +
			"and DESIGN.md). You do not normally need to run this: jl creates the directory\n" +
			"on first use. Idempotent: existing files are never overwritten.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			if err := s.Init(); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(map[string]string{"data_dir": s.Dir, "status": "initialized"})
			}
			info("Initialized jl data directory at %s", s.Dir)
			return nil
		},
	}
}
