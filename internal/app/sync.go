package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/domainlist"
	"github.com/neiromaster/domsync/internal/report"
	"github.com/neiromaster/domsync/internal/sshclient"
)

type Runner interface {
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFileAtomic(ctx context.Context, path string, data []byte, opts sshclient.WriteOptions) (sshclient.WriteResult, error)
	Run(ctx context.Context, command string, timeout time.Duration) (sshclient.CommandResult, error)
	Close() error
}

type SyncOptions struct {
	ConfigPath string
	Strict     bool
	DryRun     bool
}

const maxLocalSize = 10 << 20

func Sync(ctx context.Context, opts SyncOptions, dial func(context.Context, config.Config) (Runner, error)) (*report.Report, error) {
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	localBytes, err := readLocal(cfg.Source.Domains)
	if err != nil {
		return nil, err
	}
	localFile, err := domainlist.ParseLocal(localBytes, cfg.Source.Domains)
	if err != nil {
		return nil, withCode(2, err)
	}
	if len(localFile.Flat) == 0 && len(localFile.Sections) == 0 {
		return nil, withCode(2, fmt.Errorf("%s: local domain list is empty", cfg.Source.Domains))
	}
	runner, err := dial(ctx, *cfg)
	if err != nil {
		return nil, withCode(3, err)
	}
	defer runner.Close()

	remoteBytes, err := runner.ReadFile(ctx, cfg.Target.Path)
	if err != nil {
		if !errors.Is(err, sshclient.ErrNotExist) {
			return nil, withCode(3, err)
		}
		remoteBytes = []byte{}
	}
	remote, err := domainlist.ParseRemote(remoteBytes, cfg.Target.Path)
	if err != nil {
		return nil, withCode(2, err)
	}
	plan, err := domainlist.BuildPlan(domainlist.Input{
		Local:        localFile,
		Remote:       remote,
		OtherSection: cfg.Target.OtherSection,
	})
	if err != nil {
		return nil, withCode(2, err)
	}
	if opts.Strict && len(plan.Warnings) > 0 {
		return nil, withCode(2, errors.New(strings.Join(plan.Warnings, "; ")))
	}

	renderedRemote := domainlist.RenderRemote(plan)
	renderedLocal := domainlist.RenderLocal(plan)
	routerChanged := !bytes.Equal(renderedRemote, remoteBytes)
	localChanged := !bytes.Equal(renderedLocal, localBytes)

	rep := &report.Report{
		OK:       true,
		Changed:  routerChanged || localChanged,
		DryRun:   opts.DryRun,
		Total:    plan.Count(),
		Sections: plan.SectionCount(),
		Router: &report.FileDelta{
			Path:    cfg.Target.Path,
			Changed: routerChanged,
			Skipped: opts.DryRun && routerChanged,
			Added:   plan.RouterAdded,
			Removed: plan.RouterRemoved,
		},
		Local: &report.FileDelta{
			Path:    cfg.Source.Domains,
			Changed: localChanged,
			Skipped: opts.DryRun && localChanged,
			Added:   plan.LocalAdded,
			Removed: plan.LocalRemoved,
			Moved:   reportMoves(plan.LocalMoved),
		},
		Warnings: plan.Warnings,
		Commands: []report.Command{},
	}
	if opts.DryRun {
		return rep, nil
	}

	if routerChanged {
		result, err := runner.WriteFileAtomic(ctx, cfg.Target.Path, renderedRemote, sshclient.WriteOptions{
			Mode:         cfg.Target.FileMode(),
			Backup:       cfg.Target.BackupEnabled(),
			BackupSuffix: cfg.Target.BackupSuffix,
		})
		if err != nil {
			rep.OK = false
			return rep, withCode(3, err)
		}
		rep.Router.Backup = result.BackupPath
	}
	if localChanged {
		if err := writeLocalAtomic(cfg.Source.Domains, renderedLocal); err != nil {
			rep.OK = false
			return rep, withCode(1, err)
		}
	}

	if err := runPostWrite(ctx, runner, cfg, rep, routerChanged); err != nil {
		rep.OK = false
		if logErr := appendLog(cfg, rep); logErr != nil {
			return rep, withCode(1, logErr)
		}
		return rep, err
	}
	if err := appendLog(cfg, rep); err != nil {
		rep.OK = false
		return rep, withCode(1, err)
	}
	return rep, nil
}

func readLocal(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLocalSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxLocalSize {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxLocalSize)
	}
	return data, nil
}

func runPostWrite(ctx context.Context, runner Runner, cfg *config.Config, rep *report.Report, routerChanged bool) error {
	post := cfg.Target.PostWrite
	if post.Run == "never" {
		return nil
	}
	if post.Run != "always" && !routerChanged {
		return nil
	}
	timeout := time.Duration(post.Timeout)
	for _, command := range post.Commands {
		start := time.Now()
		result, err := runner.Run(ctx, command, timeout)
		rep.Commands = append(rep.Commands, report.Command{
			Command:    command,
			ExitCode:   result.ExitCode,
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			DurationMS: time.Since(start).Milliseconds(),
		})
		if err != nil {
			return withCode(5, fmt.Errorf("command %q: %w", command, err))
		}
		if result.ExitCode != 0 {
			return withCode(5, fmt.Errorf("command %q: exit %d: %s", command, result.ExitCode, strings.TrimSpace(result.Stderr)))
		}
	}
	return nil
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
	file, err := os.OpenFile(cfg.Logging.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(logLine(rep))
	return err
}

func logLine(rep *report.Report) string {
	fields := []string{
		time.Now().Format(time.RFC3339),
		"ok=" + strconv.FormatBool(rep.OK),
		"changed=" + strconv.FormatBool(rep.Changed),
		"total=" + strconv.Itoa(rep.Total),
		"sections=" + strconv.Itoa(rep.Sections),
		"router_added=" + strconv.Itoa(len(rep.Router.Added)),
		"router_removed=" + strconv.Itoa(len(rep.Router.Removed)),
		"local_added=" + strconv.Itoa(len(rep.Local.Added)),
		"local_removed=" + strconv.Itoa(len(rep.Local.Removed)),
		"moved=" + strconv.Itoa(len(rep.Local.Moved)),
		"commands=" + strconv.Itoa(len(rep.Commands)),
	}
	return strings.Join(fields, " ") + "\n"
}
