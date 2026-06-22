package cli

import (
	"github.com/spf13/cobra"
)

func newRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a log entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			log, err := s.LoadLog()
			if err != nil {
				return err
			}
			idx, err := findEntry(log, args[0])
			if err != nil {
				return err
			}
			removed := log[idx]
			log = append(log[:idx], log[idx+1:]...)
			if err := s.SaveLog(log); err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(removed)
			}
			info("Removed %s (%s)", removed.ID, employerTitle(removed))
			return nil
		},
	}
}
