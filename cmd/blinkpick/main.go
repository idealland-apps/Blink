package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `Blink — random reading picks for the time between tasks.

Usage:
  blinkpick [selection flags]       Open the interactive reading card.
  blinkpick suggest [flags]         Emit a noninteractive recommendation.
  blinkpick config [flags]          Configure a content provider.
  blinkpick doctor                  Check configuration and provider access.

Run "blinkpick --help" for this help. Configuration and providers are added in the first implementation milestone.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "blinkpick: %q is not implemented yet\n", args[0])
	return 2
}
