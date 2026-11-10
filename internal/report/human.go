package report

import (
	"fmt"
	"io"
	"time"
)

func Human(w io.Writer, r *Report) {
	for _, server := range r.Servers {
		writeServer(w, server, r.DryRun)
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "  предупреждение: %s\n", warning)
	}
	if r.Changed {
		fmt.Fprintf(w, "domsync: %s, %s\n",
			count(r.Total, "домен", "домена", "доменов"),
			count(r.Sections, "секция", "секции", "секций"))
	} else if r.OK && r.Error == nil {
		fmt.Fprintf(w, "domsync: изменений нет (%s, %s)\n",
			count(r.Total, "домен", "домена", "доменов"),
			count(r.Sections, "секция", "секции", "секций"))
	}
}

func writeServer(w io.Writer, server ServerResult, dryRun bool) {
	if server.Skipped {
		fmt.Fprintf(w, "%s: пропущен (disabled)\n", server.Name)
		return
	}
	if server.Error != nil {
		fmt.Fprintf(w, "%s: ошибка: %s (пропущено пар: %d)\n", server.Name, server.Error.Message, server.SkippedPairs)
		return
	}
	if server.Host != "" {
		fmt.Fprintf(w, "%s (%s)\n", server.Name, server.Host)
	} else {
		fmt.Fprintln(w, server.Name)
	}
	for _, file := range server.Files {
		writeFileResult(w, file, dryRun)
	}
	for _, command := range server.Commands {
		writeCommand(w, command)
	}
}

func writeFileResult(w io.Writer, file FileResult, dryRun bool) {
	routerChanged := file.Router != nil && file.Router.Changed
	localChanged := file.Local != nil && file.Local.Changed
	if !routerChanged && !localChanged {
		fmt.Fprintf(w, "  %s → %s: без изменений\n", file.Source, file.Target)
	} else {
		fmt.Fprintf(w, "  %s → %s\n", file.Source, file.Target)
		if routerChanged {
			writeFileDelta(w, "    ", "роутер", file.Router, false, dryRun)
		}
		if localChanged {
			writeFileDelta(w, "    ", "локально", file.Local, true, dryRun)
		}
	}
	if file.Error != nil {
		fmt.Fprintf(w, "    ошибка: %s\n", file.Error.Message)
	}
	for _, warning := range file.Warnings {
		fmt.Fprintf(w, "    предупреждение: %s\n", warning)
	}
}

func writeCommand(w io.Writer, command Command) {
	switch {
	case command.Error != "":
		fmt.Fprintf(w, "  команда: %s — ошибка: %s\n", command.Command, command.Error)
	case command.ExitCode == 0:
		fmt.Fprintf(w, "  команда: %s — ok, %s\n", command.Command, commandDuration(command.DurationMS))
	default:
		fmt.Fprintf(w, "  команда: %s — ошибка %d\n", command.Command, command.ExitCode)
	}
}

func writeFileDelta(w io.Writer, indent, label string, delta *FileDelta, local, dryRun bool) {
	if !delta.Changed {
		fmt.Fprintf(w, "%s%s: без изменений\n", indent, label)
		return
	}
	skipped := dryRun || delta.Skipped
	fmt.Fprintf(w, "%s%s: +%d −%d", indent, label, len(delta.Added), len(delta.Removed))
	if !local && delta.Written {
		fmt.Fprint(w, ", записан")
	}
	if local && len(delta.Moved) > 0 {
		fmt.Fprintf(w, ", перемещён %d", len(delta.Moved))
	}
	if skipped {
		fmt.Fprint(w, ", будет записан (dry-run)")
	}
	if delta.Backup != "" {
		fmt.Fprintf(w, ", бэкап %s", delta.Backup)
	}
	fmt.Fprintln(w)
}

func count(n int, one, few, many string) string {
	return fmt.Sprintf("%d %s", n, plural(n, one, few, many))
}

func plural(n int, one, few, many string) string {
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

func commandDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
