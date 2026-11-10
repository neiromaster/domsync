package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/update"
	"github.com/neiromaster/domsync/internal/version"
)

var (
	detectInstall = update.DetectInstall
	newChecker    = update.NewChecker
	applyUpdate   = update.Apply
)

const (
	updateBaseURL = "https://api.github.com"
	updateRepo    = "neiromaster/domsync"
)

func runUpdate(ctx context.Context, cli CLI, stdout, stderr io.Writer) int {
	execPath, err := update.ResolveExecutable()
	if err != nil {
		return updateFailure(cli, stdout, stderr, "domsync: "+err.Error())
	}
	switch detectInstall(execPath) {
	case update.InstallBrew:
		return updateFailure(cli, stdout, stderr, "domsync установлен через Homebrew. Обновление: brew upgrade --cask domsync")
	case update.InstallNPM:
		return updateFailure(cli, stdout, stderr, "domsync установлен через npm. Обновление: npm i -g @domsync/cli@latest")
	}
	if cli.Update != nil && cli.Update.Check {
		return runUpdateCheck(ctx, cli, stdout, stderr)
	}
	return runUpdateApply(ctx, cli, execPath, stdout, stderr)
}

func runUpdateCheck(ctx context.Context, cli CLI, stdout, stderr io.Writer) int {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := newChecker().Check(checkCtx, version.Version)
	if err != nil {
		return updateFailure(cli, stdout, stderr, "domsync: "+err.Error())
	}
	if cli.JSON {
		return updateSuccessReport(cli, stdout, stderr, &report.UpdateInfo{
			Current:   info.Current,
			Latest:    info.Latest,
			Available: info.Available,
		})
	}
	fmt.Fprintf(stdout, "Текущая версия: %s\n", info.Current)
	if info.Available {
		fmt.Fprintf(stdout, "Доступна %s\n", info.Latest)
		return 0
	}
	fmt.Fprintln(stdout, "Обновлений нет")
	return 0
}

func runUpdateApply(ctx context.Context, cli CLI, execPath string, stdout, stderr io.Writer) int {
	tag, err := applyUpdate(ctx, update.ApplyOptions{
		BaseURL: updateBaseURL,
		Repo:    updateRepo,
		Current: version.Version,
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		Target:  execPath,
	})
	if err != nil {
		return updateFailure(cli, stdout, stderr, "domsync: "+err.Error())
	}
	if cli.JSON {
		return updateSuccessReport(cli, stdout, stderr, &report.UpdateInfo{
			Current:   tag,
			Latest:    tag,
			Available: false,
		})
	}
	fmt.Fprintf(stdout, "domsync обновлён до %s\n", tag)
	return 0
}

func updateSuccessReport(cli CLI, stdout, stderr io.Writer, info *report.UpdateInfo) int {
	rep := &report.Report{
		OK:       true,
		Warnings: []string{},
		Update:   info,
	}
	if err := report.JSON(stdout, rep); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func updateFailure(cli CLI, stdout, stderr io.Writer, message string) int {
	fmt.Fprintln(stderr, message)
	if cli.JSON {
		if err := report.JSON(stdout, report.Failure(4, errors.New(message))); err != nil {
			fmt.Fprintln(stderr, err)
		}
	}
	return 4
}
