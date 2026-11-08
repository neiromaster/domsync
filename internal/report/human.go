package report

import (
	"fmt"
	"io"
	"time"
)

func Human(w io.Writer, r *Report) {
	if r.Changed {
		fmt.Fprintf(w, "domsync: %s, %s\n",
			count(r.Total, "домен", "домена", "доменов"),
			count(r.Sections, "секция", "секции", "секций"))
	} else if r.OK && r.Error == nil {
		fmt.Fprintf(w, "domsync: изменений нет (%s, %s)\n",
			count(r.Total, "домен", "домена", "доменов"),
			count(r.Sections, "секция", "секции", "секций"))
	}
	if r.Router != nil {
		writeFileDelta(w, "роутер", r.Router, false, r.DryRun)
	}
	if r.Local != nil {
		writeFileDelta(w, "локально", r.Local, true, r.DryRun)
	}
	for _, command := range r.Commands {
		if command.ExitCode == 0 {
			fmt.Fprintf(w, "  команда: %s — ok, %s\n", command.Command, commandDuration(command.DurationMS))
		} else {
			fmt.Fprintf(w, "  команда: %s — ошибка %d\n", command.Command, command.ExitCode)
		}
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "  предупреждение: %s\n", warning)
	}
}

func writeFileDelta(w io.Writer, label string, delta *FileDelta, local, dryRun bool) {
	if !delta.Changed {
		fmt.Fprintf(w, "  %s: без изменений\n", label)
		return
	}
	skipped := dryRun || delta.Skipped
	fmt.Fprintf(w, "  %s: +%d −%d", label, len(delta.Added), len(delta.Removed))
	if !local && !skipped {
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
