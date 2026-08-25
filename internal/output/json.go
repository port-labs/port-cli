package output

import "encoding/json"

// JSONResult represents a structured result for JSON output.
type JSONResult struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// PrintJSON prints data as JSON to stdout.
func PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(outputWriter)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// PrintJSONResult prints a JSONResult as JSON.
func PrintJSONResult(result JSONResult) error {
	return PrintJSON(result)
}
