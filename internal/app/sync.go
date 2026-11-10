package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/domainlist"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/sshclient"
	"github.com/neiromaster/domsync/internal/update"
	"github.com/neiromaster/domsync/internal/version"
)

type Runner interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFileAtomic(ctx context.Context, path string, data []byte, opts sshclient.WriteOptions) (sshclient.WriteResult, error)
	Run(ctx context.Context, command string, timeout time.Duration) (sshclient.CommandResult, error)
	Close() error
}

type SyncOptions struct {
	ConfigPath  string
	Strict      bool
	DryRun      bool
	CheckUpdate bool
	Verbose     bool
	Checker     *update.Checker
}

const maxLocalSize = 10 << 20

func Sync(ctx context.Context, opts SyncOptions, dial func(context.Context, config.Server) (Runner, error)) (*report.Report, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	lock, err := acquireRunLock(lockPath(cfg.Path()))
	if err != nil {
		return nil, withCode(1, err)
	}
	defer func() { _ = lock.release() }()

	rep := &report.Report{
		OK:       true,
		DryRun:   opts.DryRun,
		Servers:  make([]report.ServerResult, 0, len(cfg.Servers)),
		Warnings: []string{},
	}
	var firstErr error
	for _, srv := range cfg.Servers {
		result, err := syncServer(ctx, opts, srv, dial)
		rep.Servers = append(rep.Servers, result)
		rep.OK = rep.OK && result.OK
		rep.Changed = rep.Changed || result.Changed
		for _, file := range result.Files {
			rep.Total += file.Total
			rep.Sections += file.Sections
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if !opts.DryRun {
		if logErr := appendLog(cfg, rep); logErr != nil {
			rep.OK = false
			if firstErr == nil {
				firstErr = withCode(1, withRunLevel(logErr))
			}
			return rep, firstErr
		}
	}
	checkForUpdate(ctx, opts, cfg, rep)
	return rep, firstErr
}

func syncServer(ctx context.Context, opts SyncOptions, srv config.Server, dial func(context.Context, config.Server) (Runner, error)) (report.ServerResult, error) {
	result := report.ServerResult{
		Name:     srv.Name,
		Host:     srv.Host,
		OK:       true,
		Files:    []report.FileResult{},
		Commands: []report.Command{},
	}
	if !srv.IsEnabled() {
		result.Skipped = true
		return result, nil
	}

	runner, err := dial(ctx, srv)
	if err != nil {
		result.OK = false
		result.SkippedPairs = len(srv.Files)
		result.Error = &report.ErrorInfo{Code: 3, Message: err.Error()}
		return result, withCode(3, err)
	}
	defer func() { _ = runner.Close() }()

	var firstErr error
	for _, pair := range srv.Files {
		file, err := syncPair(ctx, runner, pair, opts.DryRun, opts.Strict)
		result.Files = append(result.Files, *file)
		result.Changed = result.Changed || file.Changed
		if err != nil {
			result.OK = false
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if !opts.DryRun && runsPostWrite(srv.PostWrite, result.Changed) {
		if err := runPostWrite(ctx, runner, srv.PostWrite, &result.Commands); err != nil {
			result.OK = false
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return result, firstErr
}

func runsPostWrite(post config.PostWrite, changed bool) bool {
	switch post.Run {
	case "always":
		return true
	case "on_change":
		return changed
	default:
		return false
	}
}

func syncPair(ctx context.Context, runner Runner, pair config.Pair, dryRun, strict bool) (*report.FileResult, error) {
	result := &report.FileResult{
		Source:   pair.Source,
		Target:   pair.Target,
		Warnings: []string{},
	}
	fail := func(code int, err error) (*report.FileResult, error) {
		result.OK = false
		result.Error = &report.ErrorInfo{Code: code, Message: err.Error()}
		return result, withCode(code, err)
	}

	localBytes, err := readLocal(pair.Source)
	if err != nil {
		return fail(1, err)
	}
	localFile, err := domainlist.ParseLocal(localBytes, pair.Source)
	if err != nil {
		return fail(2, err)
	}
	remoteBytes, err := runner.ReadFile(ctx, pair.Target)
	if err != nil {
		if !errors.Is(err, sshclient.ErrNotExist) {
			return fail(3, err)
		}
		remoteBytes = []byte{}
	}
	remote, err := domainlist.ParseRemote(remoteBytes, pair.Target)
	if err != nil {
		return fail(2, err)
	}
	plan, err := domainlist.BuildPlan(domainlist.Input{
		Local:        localFile,
		Remote:       remote,
		OtherSection: pair.OtherSection,
	})
	if err != nil {
		return fail(2, err)
	}
	result.Warnings = plan.Warnings
	if strict && len(plan.Warnings) > 0 {
		return fail(2, errors.New(strings.Join(plan.Warnings, "; ")))
	}

	renderedRemote := domainlist.RenderRemote(plan)
	renderedLocal := domainlist.RenderLocal(plan)
	routerChanged := !bytes.Equal(renderedRemote, remoteBytes)
	localChanged := !bytes.Equal(renderedLocal, localBytes)

	result.OK = true
	result.Changed = routerChanged || localChanged
	result.Total = plan.Count()
	result.Sections = plan.SectionCount()
	result.Router = &report.FileDelta{
		Path:    pair.Target,
		Changed: routerChanged,
		Skipped: dryRun && routerChanged,
		Added:   plan.RouterAdded,
		Removed: plan.RouterRemoved,
	}
	result.Local = &report.FileDelta{
		Path:    pair.Source,
		Changed: localChanged,
		Skipped: dryRun && localChanged,
		Added:   plan.LocalAdded,
		Removed: plan.LocalRemoved,
		Moved:   reportMoves(plan.LocalMoved),
	}
	if dryRun {
		return result, nil
	}

	if routerChanged {
		written, err := runner.WriteFileAtomic(ctx, pair.Target, renderedRemote, sshclient.WriteOptions{
			Mode:         pair.FileMode(),
			Backup:       pair.BackupEnabled(),
			BackupSuffix: pair.BackupSuffix,
		})
		if err != nil {
			return fail(3, err)
		}
		result.Router.Backup = written.BackupPath
		result.Router.Written = true
	}
	if localChanged {
		if err := writeLocalAtomic(pair.Source, renderedLocal); err != nil {
			return fail(1, err)
		}
		result.Local.Written = true
	}
	return result, nil
}

func runPostWrite(ctx context.Context, runner Runner, post config.PostWrite, commands *[]report.Command) error {
	timeout := time.Duration(post.Timeout)
	for _, command := range post.Commands {
		start := time.Now()
		result, err := runner.Run(ctx, command, timeout)
		entry := report.Command{
			Command:    command,
			ExitCode:   result.ExitCode,
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			DurationMS: time.Since(start).Milliseconds(),
		}
		if err != nil {
			entry.Error = err.Error()
		}
		*commands = append(*commands, entry)
		if err != nil {
			return withCode(5, fmt.Errorf("command %q: %w", command, err))
		}
		if result.ExitCode != 0 {
			return withCode(5, fmt.Errorf("command %q: exit %d: %s", command, result.ExitCode, strings.TrimSpace(result.Stderr)))
		}
	}
	return nil
}

func checkForUpdate(ctx context.Context, opts SyncOptions, cfg *config.Config, rep *report.Report) {
	if !opts.CheckUpdate || opts.Checker == nil || !cfg.Update.CheckEnabled() {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	info, err := opts.Checker.Check(checkCtx, version.Version)
	if err != nil {
		if opts.Verbose {
			rep.Warnings = append(rep.Warnings, "проверка обновления: "+err.Error())
		}
		return
	}
	if info.Available {
		rep.Update = &report.UpdateInfo{
			Current:   info.Current,
			Latest:    info.Latest,
			Available: true,
		}
	}
}

func readLocal(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // path is the user-configured domain list location
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxLocalSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxLocalSize {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxLocalSize)
	}
	return data, nil
}

func reportMoves(moves []domainlist.Move) []report.Move {
	result := make([]report.Move, 0, len(moves))
	for _, move := range moves {
		result = append(result, report.Move{Pattern: move.Pattern, From: move.From, To: move.To})
	}
	return result
}

func appendLog(cfg *config.Config, rep *report.Report) error {
	if cfg.Logging.File == "" {
		return nil
	}
	file, err := os.OpenFile(cfg.Logging.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // log file stays group/world-readable by design
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.WriteString(logLine(rep))
	return err
}

func logLine(rep *report.Report) string {
	counts := sumDeltas(rep)
	fields := []string{
		time.Now().Format(time.RFC3339),
		"ok=" + strconv.FormatBool(rep.OK),
		"changed=" + strconv.FormatBool(rep.Changed),
		"total=" + strconv.Itoa(rep.Total),
		"sections=" + strconv.Itoa(rep.Sections),
		"servers=" + serverFlags(rep),
		"router_added=" + strconv.Itoa(counts.routerAdded),
		"router_removed=" + strconv.Itoa(counts.routerRemoved),
		"local_added=" + strconv.Itoa(counts.localAdded),
		"local_removed=" + strconv.Itoa(counts.localRemoved),
		"moved=" + strconv.Itoa(counts.moved),
		"commands=" + strconv.Itoa(counts.commands),
	}
	return strings.Join(fields, " ") + "\n"
}

type deltaCounts struct {
	routerAdded   int
	routerRemoved int
	localAdded    int
	localRemoved  int
	moved         int
	commands      int
}

func sumDeltas(rep *report.Report) deltaCounts {
	var counts deltaCounts
	for _, server := range rep.Servers {
		counts.commands += len(server.Commands)
		for _, file := range server.Files {
			if file.Router != nil {
				counts.routerAdded += len(file.Router.Added)
				counts.routerRemoved += len(file.Router.Removed)
			}
			if file.Local != nil {
				counts.localAdded += len(file.Local.Added)
				counts.localRemoved += len(file.Local.Removed)
				counts.moved += len(file.Local.Moved)
			}
		}
	}
	return counts
}

func serverFlags(rep *report.Report) string {
	flags := make([]string, 0, len(rep.Servers))
	for _, server := range rep.Servers {
		state := "ok"
		switch {
		case server.Skipped:
			state = "skipped"
		case !server.OK:
			state = "fail"
		}
		flags = append(flags, server.Name+"="+state)
	}
	return strings.Join(flags, ",")
}
