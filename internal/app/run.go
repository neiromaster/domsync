package app

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/neiromaster/domsync/internal/version"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = stdin
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("domsync"),
		kong.Description("Sync a local domain list to an OpenClash rule-provider file."),
		kong.Writers(stdout, stderr),
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_, err = parser.Parse(args)
	// Kong validates before returning, so a --version-only invocation fails with a
	// missing-command error; flags are applied before validation, so cli.Version is set.
	if err != nil && !cli.Version {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if cli.Version {
		fmt.Fprintf(stdout, "domsync %s\n", version.String())
		return 0
	}
	fmt.Fprintln(stderr, "domsync: not implemented")
	return 1
}
