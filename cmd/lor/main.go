// Command lor is the agent CLI: request temporary SSH access through Lor and
// run commands with `lor ssh` (plan section 10).
package main

import (
	"fmt"
	"os"

	"github.com/joshualent/lor/internal/cli"
)

func main() {
	app, err := cli.NewApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lor:", err)
		os.Exit(1)
	}
	os.Exit(app.Run(os.Args[1:]))
}
