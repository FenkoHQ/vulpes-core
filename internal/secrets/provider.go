package secrets

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var refRE = regexp.MustCompile(`^\$\{secret:([A-Za-z0-9_./-]+)\}$`)

type Provider interface {
	Resolve(name string) (string, error)
}

type EnvProvider struct{ Enabled bool }

func (p EnvProvider) Resolve(name string) (string, error) {
	if !p.Enabled {
		return "", fmt.Errorf("environment secrets disabled")
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("secret %s not found in environment", name)
	}
	return v, nil
}

func IsSecretRef(s string) (string, bool) {
	m := refRE.FindStringSubmatch(s)
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}

func ResolvePluginSecrets(config map[string]any, permitted []string, p Provider) (map[string]any, map[string]string, error) {
	allowed := map[string]bool{}
	for _, name := range permitted {
		allowed[name] = true
	}
	resolvedSecrets := map[string]string{}
	out, err := resolveValue(config, allowed, p, resolvedSecrets)
	if err != nil {
		return nil, nil, err
	}
	m, _ := out.(map[string]any)
	return m, resolvedSecrets, nil
}

func resolveValue(v any, allowed map[string]bool, p Provider, secrets map[string]string) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			r, err := resolveValue(v, allowed, p, secrets)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			r, err := resolveValue(v, allowed, p, secrets)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case string:
		name, ok := IsSecretRef(strings.TrimSpace(x))
		if !ok {
			return x, nil
		}
		if !allowed[name] {
			return nil, fmt.Errorf("secret %s not permitted for plugin", name)
		}
		val, err := p.Resolve(name)
		if err != nil {
			return nil, err
		}
		secrets[name] = val
		return val, nil
	default:
		return v, nil
	}
}
