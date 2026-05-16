package config

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!str" {
			p, err := time.ParseDuration(node.Value)
			if err != nil {
				return err
			}
			*d = Duration(p)
			return nil
		}
		n, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return err
		}
		*d = Duration(time.Duration(n))
		return nil
	default:
		return fmt.Errorf("invalid duration node %s", node.Value)
	}
}

func (d Duration) Duration() time.Duration      { return time.Duration(d) }
func (d Duration) MarshalText() ([]byte, error) { return []byte(time.Duration(d).String()), nil }
