package config

import (
	"errors"
	"fmt"
)

func Validate(c *Config) error {
	if c.Server.Listen == "" {
		return errors.New("server.listen is required")
	}
	seen := map[string]bool{}
	for i, p := range c.Plugins {
		if p.Name == "" {
			return fmt.Errorf("plugins[%d].name is required", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate plugin name %q", p.Name)
		}
		seen[p.Name] = true
		if p.Source.Type == "" {
			return fmt.Errorf("plugin %q source.type is required", p.Name)
		}
		switch p.Source.Type {
		case "filesystem":
			if p.Source.Path == "" {
				return fmt.Errorf("plugin %q source.path is required", p.Name)
			}
		case "github":
			if p.Source.Repository == "" || p.Source.Release == "" || p.Source.Asset == "" || p.Source.Binary == "" {
				return fmt.Errorf("plugin %q github source requires repository, release, asset, and binary", p.Name)
			}
		default:
			return fmt.Errorf("plugin %q unsupported source type %q", p.Name, p.Source.Type)
		}
		if p.FailMode != "open" && p.FailMode != "closed" {
			return fmt.Errorf("plugin %q fail_mode must be open or closed", p.Name)
		}
		if p.Trust != "trusted" && p.Trust != "community" && p.Trust != "untrusted" {
			return fmt.Errorf("plugin %q trust must be trusted, community, or untrusted", p.Name)
		}
	}
	return nil
}
