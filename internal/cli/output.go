package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

// emitJSON prints v as indented JSON to stdout. Commands call this when the
// global --json flag is set.
func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// newTabWriter returns a tabwriter on stdout for aligned table output. Callers
// write tab-separated rows and call Flush.
func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
}

// info prints a human message to stderr so it never pollutes piped stdout.
func info(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
}
