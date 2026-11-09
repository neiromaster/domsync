package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/neiromaster/domsync/internal/config"
	"github.com/neiromaster/domsync/internal/domainlist"
	"github.com/neiromaster/domsync/internal/report"
)

type validateOutput struct {
	OK       bool              `json:"ok"`
	Config   string            `json:"config,omitempty"`
	Domains  string            `json:"domains,omitempty"`
	Total    int               `json:"total"`
	Sections int               `json:"sections"`
	Error    *report.ErrorInfo `json:"error,omitempty"`
}

func runValidate(cli CLI, stdout, stderr io.Writer) int {
	asJSON := cli.JSON
	res, err := validate(cli.Config)
	if err != nil {
		code := ExitCode(err)
		if asJSON {
			if encErr := writeValidateJSON(stdout, &validateOutput{Error: &report.ErrorInfo{Code: code, Message: err.Error()}}); encErr != nil {
				fmt.Fprintln(stderr, encErr)
			}
		}
		fmt.Fprintln(stderr, "domsync:", err)
		return code
	}
	if asJSON {
		if err := writeValidateJSON(stdout, res); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "domsync: конфигурация и локальный список в порядке (доменов: %d, секций: %d)\n", res.Total, res.Sections)
	return 0
}

func validate(configPath string) (*validateOutput, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	data, err := readLocal(cfg.Source.Domains)
	if err != nil {
		return nil, err
	}
	local, err := domainlist.ParseLocal(data, cfg.Source.Domains)
	if err != nil {
		return nil, withCode(2, err)
	}
	total, sections := localCounts(local)
	return &validateOutput{
		OK:       true,
		Config:   cfg.Path(),
		Domains:  cfg.Source.Domains,
		Total:    total,
		Sections: sections,
	}, nil
}

func localCounts(file *domainlist.LocalFile) (total, sections int) {
	for _, section := range file.Sections {
		if len(section.Entries) == 0 {
			continue
		}
		sections++
		total += len(section.Entries)
	}
	total += len(file.Flat)
	return total, sections
}

func writeValidateJSON(w io.Writer, res *validateOutput) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(res)
}
