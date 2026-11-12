// Command preview renders UI cases from internal/cli with fixtures.
// It is a development tool and is not shipped in the emobase release.
package main

import (
	"os"

	"github.com/EMOBase/emobase-cli/internal/cli"
)

func main() {
	os.Exit(cli.RunPreview(os.Args[1:], os.Stdout, os.Stderr))
}
