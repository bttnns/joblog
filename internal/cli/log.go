package cli

import "github.com/spf13/cobra"

func init() {
	addCommand(newLogCmd)
	// Keep the daily-path verbs reachable at the top level (jl add, jl list,
	// jl update, jl rm) as hidden aliases of their jl log counterparts, so the
	// regroup does not break muscle memory.
	addCommand(hiddenAlias(newAddCmd))
	addCommand(hiddenAlias(newListCmd))
	addCommand(hiddenAlias(newUpdateCmd))
	addCommand(hiddenAlias(newRmCmd))
}

// newLogCmd groups the work-search log verbs under `jl log`. A bare `jl log`
// runs ls.
func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Your work-search log: applications and activities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogList(cmd)
		},
	}
	cmd.AddCommand(newAddCmd(), newListCmd(), newLogShowCmd(), newUpdateCmd(), newRmCmd())
	return cmd
}

// newLogShowCmd shows one log entry's full detail.
func newLogShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one log entry's full detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			log, err := s.LoadLog()
			if err != nil {
				return err
			}
			idx, err := findEntry(log, args[0])
			if err != nil {
				return err
			}
			e := log[idx]
			if wantJSON(cmd) {
				return emitJSON(e)
			}
			printLogEntry(e)
			return nil
		},
	}
}

// hiddenAlias wraps a command constructor so the produced command is hidden from
// help. Used to expose log verbs at the top level without cluttering it.
func hiddenAlias(f func() *cobra.Command) func() *cobra.Command {
	return func() *cobra.Command {
		c := f()
		c.Hidden = true
		return c
	}
}
