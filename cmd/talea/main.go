// Talea — Trace the session. Resume the work.
package main

import (
	"os"

	"github.com/talea/talea/internal/cli"
	"github.com/talea/talea/internal/i18n"
)

func main() {
	i18n.Set(i18n.Detect())
	os.Exit(cli.Execute())
}
