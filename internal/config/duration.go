package config

import (
	"fmt"
	"time"

	"go.yaml.in/yaml/v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return fmt.Errorf("line %d: duration must be a string such as \"90s\"", node.Line)
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("line %d: invalid duration %q: %w", node.Line, value, err)
	}
	*d = Duration(parsed)
	return nil
}
