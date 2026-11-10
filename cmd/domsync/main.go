// Command domsync keeps local sectioned domain lists in sync with
// OpenClash/Mihomo rule-provider files on one or more servers over SSH.
//
// Configuration is read from domsync.yaml. The repository README documents the
// file formats, configuration keys, exit codes and the security model:
// https://github.com/neiromaster/domsync#readme
package main

import (
	"os"

	"github.com/neiromaster/domsync/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
