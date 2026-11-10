package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/update"
	"golang.org/x/term"
)

var (
	resolveExecutable = update.ResolveExecutable
	userCacheDir      = update.CacheDir
	currentGOOS       = runtime.GOOS
	isTerminal        = func(r io.Reader) bool {
		file, ok := r.(*os.File)
		return ok && term.IsTerminal(int(file.Fd()))
	}
	runExternal = func(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
		command := exec.CommandContext(ctx, name, args...) //nolint:gosec // the command comes from a fixed table of package managers
		command.Stdout = stdout
		command.Stderr = stderr
		return command.Run()
	}
)

func runUninstall(ctx context.Context, cli CLI, stdin io.Reader, stdout, stderr io.Writer) int {
	execPath, err := resolveExecutable()
	if err != nil {
		return uninstallFailure(cli, stdout, stderr, 1, "domsync: "+err.Error())
	}
	cacheDir, err := userCacheDir()
	if err != nil {
		cacheDir = ""
	}
	kind := detectInstall(execPath)
	command := uninstallPackageCommand(kind)

	removed := []string{execPath}
	if cacheDir != "" {
		removed = append(removed, cacheDir)
	}

	if cli.DryRun {
		return uninstallSuccess(cli, stdout, stderr, removed, true)
	}
	if cli.Uninstall == nil || !cli.Uninstall.Yes {
		if !isTerminal(stdin) {
			return uninstallFailure(cli, stdout, stderr, 4, "domsync: удаление без вопроса требует --yes")
		}
		fmt.Fprint(stderr, "Удалить domsync и созданные им файлы? [y/N] ")
		if !confirmedByUser(stdin) {
			return uninstallFailure(cli, stdout, stderr, 4, "domsync: удаление отменено")
		}
	}

	switch kind {
	case update.InstallBrew, update.InstallNPM:
		if kind == update.InstallNPM && currentGOOS == "windows" {
			_ = removeCacheDir(cacheDir)
			return uninstallFailure(cli, stdout, stderr, 4,
				"domsync: запущенный пакет нельзя удалить в Windows; выполните: "+strings.Join(command, " "))
		}
		packageOutput := stdout
		if cli.JSON {
			packageOutput = stderr
		}
		if err := runExternal(ctx, packageOutput, stderr, command[0], command[1:]...); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				_ = removeCacheDir(cacheDir)
				return uninstallFailure(cli, stdout, stderr, 4,
					"domsync: не найден "+command[0]+"; удалите вручную: "+strings.Join(command, " "))
			}
			return uninstallFailure(cli, stdout, stderr, 1, "domsync: "+err.Error())
		}
	default:
		if err := os.Remove(execPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return uninstallFailure(cli, stdout, stderr, 1, "domsync: "+execPath+": "+err.Error())
		}
	}

	if err := removeCacheDir(cacheDir); err != nil {
		return uninstallFailure(cli, stdout, stderr, 1, "domsync: кэш не удалён: "+err.Error())
	}
	return uninstallSuccess(cli, stdout, stderr, removed, false)
}

func uninstallPackageCommand(kind update.InstallKind) []string {
	switch kind {
	case update.InstallBrew:
		return []string{"brew", "uninstall", "--cask", "--zap", "domsync"}
	case update.InstallNPM:
		return []string{"npm", "uninstall", "-g", "@domsync/cli"}
	default:
		return nil
	}
}

func removeCacheDir(cacheDir string) error {
	if cacheDir == "" {
		return nil
	}
	return os.RemoveAll(cacheDir)
}

func confirmedByUser(r io.Reader) bool {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "д", "да":
		return true
	default:
		return false
	}
}

func uninstallSuccess(cli CLI, stdout, stderr io.Writer, removed []string, dryRun bool) int {
	if cli.JSON {
		rep := &report.Report{
			OK:       true,
			DryRun:   dryRun,
			Servers:  []report.ServerResult{},
			Warnings: []string{},
			Removed:  removed,
		}
		if err := report.JSON(stdout, rep); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if dryRun {
		fmt.Fprintln(stdout, "Будут удалены:")
	} else {
		fmt.Fprintln(stdout, "Удалено:")
	}
	for _, path := range removed {
		fmt.Fprintf(stdout, "  %s\n", path)
	}
	return 0
}

func uninstallFailure(cli CLI, stdout, stderr io.Writer, code int, message string) int {
	fmt.Fprintln(stderr, message)
	if cli.JSON {
		if err := report.JSON(stdout, report.Failure(code, errors.New(message))); err != nil {
			fmt.Fprintln(stderr, err)
		}
	}
	return code
}
