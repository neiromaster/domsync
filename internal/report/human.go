package report

import (
	"fmt"
	"io"
)

func Human(w io.Writer, r *Report) {
	if r.Changed {
		fmt.Fprintf(w, "domsync: %d доменов, %d секций\n", r.Total, r.Sections)
	} else if r.Error == nil {
		fmt.Fprintf(w, "domsync: изменений нет (%d доменов, %d секций)\n", r.Total, r.Sections)
	}
	if r.Router != nil {
		writeFileDelta(w, "роутер", r.Router, false)
	}
	if r.Local != nil {
		writeFileDelta(w, "локально", r.Local, true)
	}
	for _, command := range r.Commands {
		if command.ExitCode == 0 {
			fmt.Fprintf(w, "  команда: %s — ok, %dms\n", command.Command, command.DurationMS)
		} else {
			fmt.Fprintf(w, "  команда: %s — ошибка %d\n", command.Command, command.ExitCode)
		}
	}
	for _, warning := range r.Warnings {
		fmt.Fprintf(w, "  предупреждение: %s\n", warning)
	}
}

func writeFileDelta(w io.Writer, label string, delta *FileDelta, local bool) {
	if !delta.Changed {
		fmt.Fprintf(w, "  %s: без изменений\n", label)
		return
	}
	fmt.Fprintf(w, "  %s: +%d −%d", label, len(delta.Added), len(delta.Removed))
	if !local && !delta.Skipped {
		fmt.Fprint(w, ", записан")
	}
	if len(delta.Moved) > 0 {
		fmt.Fprintf(w, ", перемещён %d", len(delta.Moved))
	}
	if delta.Skipped {
		fmt.Fprint(w, ", будет записан (dry-run)")
	}
	if delta.Backup != "" {
		fmt.Fprintf(w, ", бэкап %s", delta.Backup)
	}
	fmt.Fprintln(w)
}
