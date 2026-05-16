package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/FenkoHQ/vulpes-core/internal/capabilities"
	"github.com/FenkoHQ/vulpes-core/internal/pipeline"
)

type errorBody struct {
	Error errorObject `json:"error"`
}
type errorObject struct {
	Type      string         `json:"type"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id,omitempty"`
	Missing   []string       `json:"missing_capabilities,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

func writeError(w http.ResponseWriter, requestID string, err error) {
	status := http.StatusInternalServerError
	obj := errorObject{Type: "gateway_error", Code: "internal_error", Message: err.Error(), RequestID: requestID}
	switch e := err.(type) {
	case pipeline.MissingCapabilitiesError:
		status = http.StatusServiceUnavailable
		obj.Type = "gateway_configuration_error"
		obj.Code = "missing_required_capabilities"
		obj.Message = e.Error()
		obj.Missing = capsToStrings(e.Missing)
	case pipeline.GatewayError:
		status = e.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		obj.Type = e.Type
		obj.Code = e.Code
		obj.Message = e.Message
		obj.Details = e.Details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: obj})
}

func capsToStrings(caps []capabilities.CapabilityType) []string {
	out := make([]string, len(caps))
	for i, c := range caps {
		out[i] = string(c)
	}
	return out
}
