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
	"github.com/neiromaster/domsync/internal/update"
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
	if ctx.Command() == "update" {
		return runUpdate(context.Background(), cli, stdout, stderr)
	}
	if ctx.Command() == "validate" {
		return runValidate(cli, stdout, stderr)
	}
	return runSync(context.Background(), cli, stdout, stderr)
}

func runSync(ctx context.Context, cli CLI, stdout, stderr io.Writer) int {
	var checker *update.Checker
	if !cli.NoUpdateCheck {
		checker = newChecker()
	}
	rep, err := Sync(ctx, SyncOptions{
		ConfigPath:  cli.Config,
		Strict:      cli.Strict,
		DryRun:      cli.DryRun,
		CheckUpdate: !cli.NoUpdateCheck,
		Verbose:     cli.Verbose,
		Checker:     checker,
	}, dialConfig)
	if err != nil {
		code := ExitCode(err)
		if rep == nil {
			rep = report.Failure(code, err)
		} else {
			rep.OK = false
			if isRunLevel(err) {
				rep.Error = &report.ErrorInfo{Code: code, Message: err.Error()}
			}
		}
		if cli.JSON {
			if encErr := report.JSON(stdout, rep); encErr != nil {
				fmt.Fprintln(stderr, encErr)
			}
		} else {
			report.Human(stderr, rep)
		}
		fmt.Fprintln(stderr, "domsync:", err)
		return code
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
		if rep.Update != nil && rep.Update.Available {
			fmt.Fprintf(stderr, "domsync: доступна %s (у вас %s). Обновление: domsync update\n",
				rep.Update.Latest, rep.Update.Current)
		}
	}
	return 0
}

func dialConfig(ctx context.Context, srv config.Server) (Runner, error) {
	return sshclient.Dial(ctx, sshclient.Options{
		Host:       srv.Host,
		Port:       srv.Port,
		User:       srv.User,
		KeyPath:    srv.SSHKey,
		KnownHosts: srv.KnownHosts,
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
