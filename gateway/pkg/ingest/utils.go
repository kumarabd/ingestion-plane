package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DetermineSchema determines the schema type based on message content and fields
func DetermineSchema(message string, fields map[string]string) string {
	// Check if it's JSON
	if strings.HasPrefix(strings.TrimSpace(message), "{") && strings.HasSuffix(strings.TrimSpace(message), "}") {
		return "JSON"
	}

	// Check if it's logfmt (key=value pairs)
	if strings.Contains(message, "=") && !strings.Contains(message, "{") {
		return "LOGFMT"
	}

	// Default to TEXT
	return "TEXT"
}

// ExtractFieldsFromJSON extracts structured fields from JSON payload
func ExtractFieldsFromJSON(payload string) map[string]string {
	fields := make(map[string]string)

	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &jsonData); err == nil {
		for k, v := range jsonData {
			if strVal, ok := v.(string); ok {
				fields[k] = strVal
			} else {
				fields[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	return fields
}

// ExtractFieldsFromLogFMT extracts structured fields from LOGFMT payload
func ExtractFieldsFromLogFMT(payload string) map[string]string {
	fields := make(map[string]string)

	// Parse key=value pairs
	pairs := strings.Fields(payload)
	for _, pair := range pairs {
		if strings.Contains(pair, "=") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				fields[parts[0]] = parts[1]
			}
		}
	}

	return fields
}

// ExtractFields extracts structured fields from payload based on schema
func ExtractFields(payload, schema string) map[string]string {
	switch schema {
	case "JSON":
		return ExtractFieldsFromJSON(payload)
	case "LOGFMT":
		return ExtractFieldsFromLogFMT(payload)
	default:
		return make(map[string]string)
	}
}
