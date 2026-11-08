package app

type CLI struct {
	Config        string `help:"Path to config file."`
	Verbose       bool   `short:"v" help:"Verbose output."`
	Quiet         bool   `short:"q" help:"Only errors."`
	JSON          bool   `help:"Machine-readable output."`
	Strict        bool   `help:"Treat local warnings as errors."`
	DryRun        bool   `name:"dry-run" help:"Compute and report without writing."`
	NoUpdateCheck bool   `name:"no-update-check" help:"Skip the update check."`
	Version       bool   `name:"version" help:"Print version and exit."`
	Help          bool   `short:"h" help:"Show help and exit."`

	Update *UpdateCmd `cmd:"" help:"Update domsync to the latest release."`
}

type UpdateCmd struct {
	Check bool `help:"Only check for a newer version."`
}

func (c *CLI) Run() error {
	return nil
}
