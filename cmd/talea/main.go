// Talea — Trace the session. Resume the work.
package main

import (
	"os"

	"github.com/talea/talea/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
