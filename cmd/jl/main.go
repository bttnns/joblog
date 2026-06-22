// Command jl is a local-first job-search tracker and unemployment work-search
// compliance tool. See DESIGN.md for the full design.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/bttnns/joblog/internal/cli"
)

// Exit codes. 1 is a tool/usage failure; 2 is the specific "ran fine but you are
// short of the weekly work-search minimum" case, so a script can tell them apart.
const (
	exitError          = 1
	exitComplianceFail = 2
)

func main() {
	err := cli.Execute()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "jl: "+err.Error())
	if errors.Is(err, cli.ErrShortOfMinimum) {
		os.Exit(exitComplianceFail)
	}
	os.Exit(exitError)
}
