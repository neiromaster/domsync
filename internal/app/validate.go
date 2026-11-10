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
	OK      bool              `json:"ok"`
	Servers []validateServer  `json:"servers"`
	Error   *report.ErrorInfo `json:"error,omitempty"`
	config  string
}

type validateServer struct {
	Name    string         `json:"name"`
	OK      bool           `json:"ok"`
	Skipped bool           `json:"skipped"`
	Files   []validateFile `json:"files"`
}

type validateFile struct {
	Source   string            `json:"source"`
	OK       bool              `json:"ok"`
	Total    int               `json:"total"`
	Sections int               `json:"sections"`
	Error    *report.ErrorInfo `json:"error,omitempty"`
}

func runValidate(cli CLI, stdout, stderr io.Writer) int {
	res, err := validate(cli.Config)
	if err != nil {
		code := ExitCode(err)
		if cli.JSON {
			failure := &validateOutput{
				Servers: []validateServer{},
				Error:   &report.ErrorInfo{Code: code, Message: err.Error()},
			}
			if encErr := writeValidateJSON(stdout, failure); encErr != nil {
				fmt.Fprintln(stderr, encErr)
			}
		}
		fmt.Fprintln(stderr, "domsync:", err)
		return code
	}
	if cli.JSON {
		if err := writeValidateJSON(stdout, res); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return res.failureCode()
	}
	writeValidateHuman(stdout, res)
	return res.failureCode()
}

func validate(configPath string) (*validateOutput, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	res := &validateOutput{
		OK:      true,
		Servers: make([]validateServer, 0, len(cfg.Servers)),
		config:  cfg.Path(),
	}
	for _, server := range cfg.Servers {
		result := validateServerResult(server)
		res.OK = res.OK && result.OK
		res.Servers = append(res.Servers, result)
	}
	return res, nil
}

func validateServerResult(server config.Server) validateServer {
	result := validateServer{Name: server.Name, OK: true, Files: []validateFile{}}
	if !server.IsEnabled() {
		result.Skipped = true
		return result
	}
	for _, pair := range server.Files {
		file := validatePairResult(pair)
		result.OK = result.OK && file.OK
		result.Files = append(result.Files, file)
	}
	return result
}

func validatePairResult(pair config.Pair) validateFile {
	result := validateFile{Source: pair.Source, OK: true}
	data, err := readLocal(pair.Source)
	if err != nil {
		return result.failed(err)
	}
	local, err := domainlist.ParseLocal(data, pair.Source)
	if err != nil {
		return result.failed(withCode(2, err))
	}
	result.Total, result.Sections = localCounts(local)
	return result
}

func (f validateFile) failed(err error) validateFile {
	f.OK = false
	f.Error = &report.ErrorInfo{Code: ExitCode(err), Message: err.Error()}
	return f
}

func (res *validateOutput) failureCode() int {
	for _, server := range res.Servers {
		for _, file := range server.Files {
			if file.Error != nil {
				return file.Error.Code
			}
		}
	}
	return 0
}

func (res *validateOutput) counts() (total, sections int) {
	for _, server := range res.Servers {
		for _, file := range server.Files {
			total += file.Total
			sections += file.Sections
		}
	}
	return total, sections
}

func writeValidateHuman(w io.Writer, res *validateOutput) {
	if res.config != "" {
		fmt.Fprintf(w, "конфигурация: %s\n", res.config)
	}
	for _, server := range res.Servers {
		if server.Skipped {
			fmt.Fprintf(w, "%s: пропущен (disabled)\n", server.Name)
			continue
		}
		fmt.Fprintln(w, server.Name)
		for _, file := range server.Files {
			if file.Error != nil {
				fmt.Fprintf(w, "  %s: ошибка: %s\n", file.Source, file.Error.Message)
				continue
			}
			fmt.Fprintf(w, "  %s: %s, %s\n", file.Source,
				validateCount(file.Total, "домен", "домена", "доменов"),
				validateCount(file.Sections, "секция", "секции", "секций"))
		}
	}
	total, sections := res.counts()
	fmt.Fprintf(w, "domsync: %s, %s\n",
		validateCount(total, "домен", "домена", "доменов"),
		validateCount(sections, "секция", "секции", "секций"))
}

func validateCount(n int, one, few, many string) string {
	return fmt.Sprintf("%d %s", n, validatePlural(n, one, few, many))
}

func validatePlural(n int, one, few, many string) string {
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
