package domainlist

import "strings"

type Plan struct {
	HasSections   bool
	Preamble      []string
	Sections      []PlanSection
	Flat          []string
	Warnings      []string
	RouterAdded   []string
	RouterRemoved []string
	LocalAdded    []string
	LocalRemoved  []string
	LocalMoved    []Move
}

type PlanSection struct {
	Name     string
	Comments []string
	Patterns []string
}

type Move struct {
	Pattern string
	From    string
	To      string
}

func RenderLocal(plan *Plan) []byte {
	lines := make([]string, 0)
	if plan.HasSections {
		lines = append(lines, plan.Preamble...)
		for _, section := range plan.Sections {
			if len(section.Patterns) == 0 {
				continue
			}
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "["+section.Name+"]")
			lines = append(lines, section.Comments...)
			lines = append(lines, section.Patterns...)
		}
	} else {
		lines = append(lines, plan.Preamble...)
		if len(lines) > 0 && len(plan.Flat) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, plan.Flat...)
	}
	return renderLines(lines)
}

func RenderRemote(plan *Plan) []byte {
	lines := make([]string, 0)
	if plan.HasSections {
		for _, section := range plan.Sections {
			if len(section.Patterns) == 0 {
				continue
			}
			lines = append(lines, "# ["+section.Name+"]")
			lines = append(lines, section.Patterns...)
		}
	} else {
		lines = append(lines, plan.Flat...)
	}
	return renderLines(lines)
}

func renderLines(lines []string) []byte {
	if len(lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (p *Plan) Count() int {
	if !p.HasSections {
		return len(p.Flat)
	}
	total := 0
	for _, section := range p.Sections {
		total += len(section.Patterns)
	}
	return total
}

func (p *Plan) SectionCount() int {
	total := 0
	for _, section := range p.Sections {
		if len(section.Patterns) > 0 {
			total++
		}
	}
	return total
}
