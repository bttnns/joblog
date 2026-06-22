// Package cli wires the cobra command tree. Each command lives in its own file
// and registers itself via addCommand in an init function, so commands can be
// added without touching this file.
package cli

import (
	"fmt"

	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

// commandFactories collects every subcommand constructor. Command files append
// to it from their init functions.
var commandFactories []func() *cobra.Command

func addCommand(f func() *cobra.Command) { commandFactories = append(commandFactories, f) }

// NewRootCmd builds the root command with all registered subcommands attached.
// The --data-dir and --json flags are persistent on the root and inherited by
// every subcommand; commands read them per-invocation via openStore and
// wantJSON, so there is no shared mutable flag state across commands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "jl",
		Short: "Track your job search and stay unemployment-compliant, from the command line",
		Long: "jl is a local-first CLI for you and your AI agent to log your work-search\n" +
			"activity, surface new and changed roles (scraped outside jl), and generate your\n" +
			"state's weekly unemployment work-search report. It does not scrape. Your data\n" +
			"stays in a local data directory; see DESIGN.md.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		// Bare `jl` (no subcommand) runs the status map. `jl --help`, `jl help`,
		// and `jl <subcommand>` are handled by cobra before this RunE is reached.
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd)
		},
	}
	root.PersistentFlags().String("data-dir", "", "where your data lives (overrides $JOBLOG_HOME and the default)")
	root.PersistentFlags().Bool("json", false, "emit JSON instead of a table")
	for _, f := range commandFactories {
		root.AddCommand(f())
	}
	return root
}

// Execute runs the root command. main calls this.
func Execute() error {
	return NewRootCmd().Execute()
}

// openStore resolves the data directory honoring the --data-dir flag on cmd
// (inherited from the root). Reading from cmd keeps flag state per-invocation.
func openStore(cmd *cobra.Command) (*store.Store, error) {
	dir, _ := cmd.Flags().GetString("data-dir")
	return store.Open(dir)
}

// openStoreForWrite opens the store and takes an exclusive lock on the data
// directory, so a mutating command's load-modify-save cycle cannot lose a
// concurrent writer's update (the human and an agent can drive the same data dir
// at once). Callers defer the returned release. Read-only commands use openStore.
func openStoreForWrite(cmd *cobra.Command) (*store.Store, func(), error) {
	s, err := openStore(cmd)
	if err != nil {
		return nil, nil, err
	}
	// Make `jl init` implicit: scaffold the data tree on first write so a human
	// never has to run init by hand. The notice prints only the first time, when
	// the tree did not exist before; an already-set-up dir stays quiet.
	created := !s.Initialized()
	if err := s.Init(); err != nil {
		return nil, nil, err
	}
	if created {
		info("Created jl data directory at %s", s.Dir)
	}
	release, err := s.Lock()
	if err != nil {
		return nil, nil, fmt.Errorf("lock data dir: %w", err)
	}
	return s, func() { _ = release() }, nil
}

// wantJSON reports whether the inherited --json flag is set on cmd.
func wantJSON(cmd *cobra.Command) bool {
	b, _ := cmd.Flags().GetBool("json")
	return b
}
