package report

import (
	"encoding/json"
	"io"
)

func JSON(w io.Writer, r *Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(normalize(r))
}

func normalize(r *Report) *Report {
	clone := *r
	if clone.Warnings == nil {
		clone.Warnings = []string{}
	}
	if clone.Commands == nil {
		clone.Commands = []Command{}
	}
	clone.Router = normalizeDelta(r.Router)
	clone.Local = normalizeDelta(r.Local)
	return &clone
}

func normalizeDelta(delta *FileDelta) *FileDelta {
	if delta == nil {
		return nil
	}
	clone := *delta
	if clone.Added == nil {
		clone.Added = []string{}
	}
	if clone.Removed == nil {
		clone.Removed = []string{}
	}
	return &clone
}
