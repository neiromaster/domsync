package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/sshclient"
	"github.com/neiromaster/domsync/internal/version"
	"golang.org/x/term"
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = stdin
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("domsync"),
		kong.Description("Sync a local domain list to an OpenClash rule-provider file."),
		kong.Writers(stdout, stderr),
		kong.NoDefaultHelp(),
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if cli.Help {
		if err := ctx.PrintUsage(false); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if cli.Version {
		fmt.Fprintf(stdout, "domsync %s\n", version.String())
		return 0
	}
	return runSync(context.Background(), cli, stdout, stderr)
}

func runSync(ctx context.Context, cli CLI, stdout, stderr io.Writer) int {
	rep, err := Sync(ctx, SyncOptions{
		ConfigPath: cli.Config,
		Strict:     cli.Strict,
		DryRun:     cli.DryRun,
	}, dialConfig)
	if err != nil {
		if rep == nil {
			rep = report.Failure(ExitCode(err), err)
		} else {
			rep.OK = false
			rep.Error = &report.ErrorInfo{Code: ExitCode(err), Message: err.Error()}
		}
		if cli.JSON {
			if err := report.JSON(stdout, rep); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else {
			report.Human(stderr, rep)
		}
		fmt.Fprintln(stderr, "domsync:", err)
		return ExitCode(err)
	}
	if cli.JSON {
		if err := report.JSON(stdout, rep); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if !cli.Quiet {
		report.Human(stdout, rep)
	}
	return 0
}

func dialConfig(ctx context.Context, cfg config.Config) (Runner, error) {
	return sshclient.Dial(ctx, sshclient.Options{
		Host:       cfg.Target.Host,
		Port:       cfg.Target.Port,
		User:       cfg.Target.User,
		KeyPath:    cfg.Target.SSHKey,
		KnownHosts: cfg.Target.KnownHosts,
		Passphrase: passphraseFromTerminal(),
		Timeout:    10 * time.Second,
	})
}

func passphraseFromTerminal() func() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	return func() (string, error) {
		fmt.Fprint(os.Stderr, "Ключевая фраза: ")
		defer fmt.Fprintln(os.Stderr)
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(password), err
	}
}
