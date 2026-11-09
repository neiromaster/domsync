// Command domsync keeps a local sectioned domain list in sync with an
// OpenClash/Mihomo rule-provider file on a router over SSH.
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
