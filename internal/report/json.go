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
	clone.Servers = normalizeServers(r.Servers)
	return &clone
}

func normalizeServers(servers []ServerResult) []ServerResult {
	if servers == nil {
		return []ServerResult{}
	}
	clone := make([]ServerResult, len(servers))
	for i, server := range servers {
		clone[i] = normalizeServer(server)
	}
	return clone
}

func normalizeServer(server ServerResult) ServerResult {
	clone := server
	clone.Files = normalizeFiles(server.Files)
	if clone.Commands == nil {
		clone.Commands = []Command{}
	}
	return clone
}

func normalizeFiles(files []FileResult) []FileResult {
	if files == nil {
		return []FileResult{}
	}
	clone := make([]FileResult, len(files))
	for i, file := range files {
		clone[i] = normalizeFile(file)
	}
	return clone
}

func normalizeFile(file FileResult) FileResult {
	clone := file
	clone.Router = normalizeDelta(file.Router)
	clone.Local = normalizeDelta(file.Local)
	if clone.Warnings == nil {
		clone.Warnings = []string{}
	}
	return clone
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
