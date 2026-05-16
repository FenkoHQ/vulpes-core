package models

import (
	"fmt"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/config"
)

type Registry struct {
	aliases map[string][]capabilities.RouteCandidate
}

func NewRegistry(cfg config.ModelsConfig) *Registry {
	r := &Registry{aliases: map[string][]capabilities.RouteCandidate{}}
	for logical, alias := range cfg.Aliases {
		for _, c := range alias.Candidates {
			weight := c.Weight
			if weight == 0 {
				weight = 100
			}
			r.aliases[logical] = append(r.aliases[logical], capabilities.RouteCandidate{
				ProviderInstance: c.Provider,
				ProviderModel:    c.Model,
				LogicalModel:     logical,
				Weight:           weight,
				Region:           c.Region,
				Healthy:          true,
				Properties:       c.Properties,
			})
		}
	}
	return r
}

func (r *Registry) Candidates(model string) ([]capabilities.RouteCandidate, error) {
	c := r.aliases[model]
	if len(c) == 0 {
		return nil, fmt.Errorf("model %q not found", model)
	}
	out := make([]capabilities.RouteCandidate, len(c))
	copy(out, c)
	return out, nil
}

func (r *Registry) List() []capabilities.ModelInfo {
	out := make([]capabilities.ModelInfo, 0, len(r.aliases))
	for m := range r.aliases {
		out = append(out, capabilities.ModelInfo{ID: m, Object: "model", OwnedBy: "gateway", Healthy: true})
	}
	return out
}
