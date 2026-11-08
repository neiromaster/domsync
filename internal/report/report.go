package report

type Report struct {
	OK       bool        `json:"ok"`
	Changed  bool        `json:"changed"`
	DryRun   bool        `json:"dry_run,omitempty"`
	Total    int         `json:"total"`
	Sections int         `json:"sections"`
	Router   *FileDelta  `json:"router,omitempty"`
	Local    *FileDelta  `json:"local,omitempty"`
	Warnings []string    `json:"warnings"`
	Commands []Command   `json:"commands"`
	Update   *UpdateInfo `json:"update,omitempty"`
	Error    *ErrorInfo  `json:"error,omitempty"`
}

type FileDelta struct {
	Path    string   `json:"path"`
	Changed bool     `json:"changed"`
	Skipped bool     `json:"skipped,omitempty"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Moved   []Move   `json:"moved,omitempty"`
	Backup  string   `json:"backup,omitempty"`
}

type Move struct {
	Pattern string `json:"pattern"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type Command struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func Failure(code int, err error) *Report {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return &Report{
		OK:       false,
		Warnings: []string{},
		Commands: []Command{},
		Error:    &ErrorInfo{Code: code, Message: message},
	}
}
