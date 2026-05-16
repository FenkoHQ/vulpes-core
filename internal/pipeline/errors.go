package pipeline

import (
	"fmt"
	"strings"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
)

type MissingCapabilitiesError struct{ Missing []capabilities.CapabilityType }

func (e MissingCapabilitiesError) Error() string {
	parts := make([]string, len(e.Missing))
	for i, m := range e.Missing {
		parts[i] = string(m)
	}
	return fmt.Sprintf("Gateway cannot serve chat completions. Missing required capabilities: %s.", strings.Join(parts, ", "))
}

type GatewayError struct {
	Type    string
	Code    string
	Message string
	Status  int
	Details map[string]any
}

func (e GatewayError) Error() string { return e.Message }
