// Command emobase is the cross-repository control point for EMOBase
// deployments.
package main

import (
	"os"

	"github.com/EMOBase/emobase-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
