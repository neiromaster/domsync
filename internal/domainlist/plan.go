package domainlist

import (
	"errors"
	"fmt"
	"sort"
)

type Input struct {
	Local        *LocalFile
	Remote       []string
	OtherSection string
}

func BuildPlan(in Input) (*Plan, error) {
	localFile := in.Local
	if localFile == nil {
		return nil, errors.New("local domain list is empty")
	}
	if localFile.HasSections && in.OtherSection == "" {
		return nil, errors.New("other_section must be set when the local file has sections")
	}

	positives := make(map[string]string, len(localFile.Flat))
	for _, section := range localFile.Sections {
		for _, pattern := range section.Entries {
			if previous, ok := positives[pattern]; ok {
				return nil, fmt.Errorf("duplicate positive pattern %q in sections %q and %q", pattern, previous, section.Name)
			}
			positives[pattern] = section.Name
		}
	}
	for _, pattern := range localFile.Flat {
		if _, ok := positives[pattern]; ok {
			return nil, fmt.Errorf("duplicate positive pattern %q", pattern)
		}
		positives[pattern] = ""
	}

	negatives := make(map[string]Negative, len(localFile.Negatives))
	for _, negative := range localFile.Negatives {
		if _, ok := negatives[negative.Pattern]; ok {
			return nil, fmt.Errorf("duplicate deletion directive: -%s", negative.Pattern)
		}
		negatives[negative.Pattern] = negative
	}

	if len(positives) == 0 && len(negatives) == 0 {
		return nil, errors.New("local domain list is empty: refusing to build a plan")
	}

	remoteSet := make(map[string]struct{}, len(in.Remote))
	for _, pattern := range in.Remote {
		remoteSet[pattern] = struct{}{}
	}

	final := make(map[string]string, len(positives)+len(remoteSet))
	for pattern, section := range positives {
		final[pattern] = section
	}
	for _, pattern := range in.Remote {
		if _, ok := final[pattern]; ok {
			continue
		}
		if _, ok := negatives[pattern]; ok {
			continue
		}
		final[pattern] = in.OtherSection
	}

	if len(final) == 0 {
		return nil, errors.New("refusing to write an empty domain list")
	}

	plan := &Plan{
		HasSections: localFile.HasSections,
		Preamble:    append([]string(nil), localFile.Preamble...),
		Warnings:    negativeWarnings(localFile, positives, remoteSet),
	}
	if localFile.HasSections {
		plan.Sections = mergeSections(localFile, in.OtherSection, final)
	} else {
		plan.Flat = sortedKeys(final)
	}

	finalSet := toSet(final)
	positiveSet := toSet(positives)
	plan.RouterAdded = subtract(finalSet, remoteSet)
	plan.RouterRemoved = subtract(remoteSet, finalSet)
	plan.LocalAdded = subtract(finalSet, positiveSet)
	plan.LocalRemoved = subtract(positiveSet, finalSet)
	plan.LocalMoved = movedPatterns(positives, negatives, remoteSet)

	return plan, nil
}

func negativeWarnings(localFile *LocalFile, positives map[string]string, remoteSet map[string]struct{}) []string {
	warnings := make([]string, 0)
	for _, negative := range localFile.Negatives {
		if section, ok := positives[negative.Pattern]; ok && section == negative.Section {
			if localFile.HasSections {
				warnings = append(warnings, fmt.Sprintf("redundant deletion directive in section %q: -%s", negative.Section, negative.Pattern))
			} else {
				warnings = append(warnings, fmt.Sprintf("redundant deletion directive in flat list: -%s", negative.Pattern))
			}
			continue
		}
		if _, ok := positives[negative.Pattern]; ok {
			continue
		}
		if _, ok := remoteSet[negative.Pattern]; ok {
			continue
		}
		warnings = append(warnings, "deletion target not found: -"+negative.Pattern)
	}
	return warnings
}

func mergeSections(localFile *LocalFile, otherSection string, final map[string]string) []PlanSection {
	bySection := make(map[string][]string, len(localFile.Sections)+1)
	for pattern, section := range final {
		bySection[section] = append(bySection[section], pattern)
	}

	sections := make([]PlanSection, 0, len(localFile.Sections)+1)
	for _, section := range localFile.Sections {
		if section.Name == otherSection {
			continue
		}
		sections = append(sections, PlanSection{
			Name:     section.Name,
			Comments: append([]string(nil), section.Comments...),
			Patterns: sortedPatterns(bySection[section.Name]),
		})
	}

	merged := PlanSection{Name: otherSection, Patterns: sortedPatterns(bySection[otherSection])}
	for _, section := range localFile.Sections {
		if section.Name == otherSection {
			merged.Comments = append([]string(nil), section.Comments...)
		}
	}
	return append(sections, merged)
}

func movedPatterns(positives map[string]string, negatives map[string]Negative, remoteSet map[string]struct{}) []Move {
	moves := make([]Move, 0)
	for pattern, section := range positives {
		if _, ok := remoteSet[pattern]; !ok {
			continue
		}
		negative, ok := negatives[pattern]
		if !ok || negative.Section == section {
			continue
		}
		moves = append(moves, Move{Pattern: pattern, From: negative.Section, To: section})
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].Pattern < moves[j].Pattern })
	return moves
}

func sortedPatterns(patterns []string) []string {
	sorted := append([]string(nil), patterns...)
	sort.Strings(sorted)
	return sorted
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toSet(values map[string]string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for key := range values {
		set[key] = struct{}{}
	}
	return set
}

func subtract(from, exclude map[string]struct{}) []string {
	result := make([]string, 0, len(from))
	for pattern := range from {
		if _, ok := exclude[pattern]; ok {
			continue
		}
		result = append(result, pattern)
	}
	sort.Strings(result)
	return result
}
