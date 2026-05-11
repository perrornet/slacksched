package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a YAML-friendly time.Duration (e.g. `12h`, `30m`).
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err == nil {
		dd, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("parse duration %q: %w", s, err)
		}
		*d = Duration(dd)
		return nil
	}
	var sec int64
	if err := n.Decode(&sec); err != nil {
		return err
	}
	*d = Duration(time.Duration(sec) * time.Second)
	return nil
}

func (d Duration) Duration() time.Duration { return time.Duration(d) }
