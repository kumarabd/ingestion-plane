package ingest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// DetermineSchema determines the schema type based on message content
func DetermineSchema(message string, fields map[string]string) string {
	trimmed := strings.TrimSpace(message)

	// If payload starts with { → attempt json.Unmarshal
	if strings.HasPrefix(trimmed, "{") {
		var jsonData interface{}
		if err := json.Unmarshal([]byte(trimmed), &jsonData); err == nil {
			return "JSON"
		}
	}

	// If it contains = pairs → mark as logfmt
	if containsLogfmtPairs(message) {
		return "LOGFMT"
	}

	// Else → treat as text
	return "TEXT"
}

// containsLogfmtPairs checks if the message contains key=value pairs typical of logfmt
func containsLogfmtPairs(message string) bool {
	// Look for patterns like key=value with optional spaces
	logfmtPattern := regexp.MustCompile(`\b\w+\s*=\s*[^\s=]+`)
	matches := logfmtPattern.FindAllString(message, -1)

	// Consider it logfmt if we find at least one key=value pair
	// and it's not JSON (doesn't start with {)
	if len(matches) > 0 && !strings.HasPrefix(strings.TrimSpace(message), "{") {
		return true
	}

	return false
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

// NormalizeMessage normalizes a message according to the specified rules
func NormalizeMessage(message string, maxBytes int) (string, bool) {
	// 1. Enforce size cap (MaxMessageBytes), mark truncated if needed
	truncated := false
	if len(message) > maxBytes {
		message = message[:maxBytes]
		truncated = true
	}

	// 2. Canonicalize whitespace (collapse multiple spaces)
	message = canonicalizeWhitespace(message)

	// 3. Normalize UTF-8 to NFC form
	message = norm.NFC.String(message)

	return message, truncated
}

// canonicalizeWhitespace collapses multiple consecutive whitespace characters into single spaces
func canonicalizeWhitespace(s string) string {
	// Replace multiple whitespace characters with single space
	whitespacePattern := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(s, " "))
}

// NormalizeRawLog normalizes a single RawLog into a NormalizedLog
func NormalizeRawLog(rawLog RawLog, maxMessageBytes int) NormalizedLog {
	// Detect schema
	schema := DetermineSchema(rawLog.Payload, nil)

	// Normalize message
	normalizedMessage, truncated := NormalizeMessage(rawLog.Payload, maxMessageBytes)

	// Apply PII redaction
	redactor := NewPIIRedactor()
	redactedMessage, redactionReport := redactor.RedactMessage(normalizedMessage)

	// Use provided timestamp or current time if not provided
	timestamp := rawLog.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return NormalizedLog{
		Timestamp: timestamp,
		Labels:    rawLog.Labels,
		Message:   redactedMessage,
		Schema:    schema,
		Sanitized: redactionReport.Applied, // Mark as sanitized if redaction was applied
		Truncated: truncated,
		Redaction: redactionReport,
	}
}
