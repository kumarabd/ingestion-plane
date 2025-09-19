package ingest

import (
	"testing"
	"time"
)

func TestPIIRedactor(t *testing.T) {
	redactor := NewPIIRedactor()

	tests := []struct {
		name     string
		input    string
		expected string
		applied  bool
		rules    []string
	}{
		{
			name:     "IPv4 address",
			input:    "User connected from 192.168.1.1",
			expected: "User connected from <ipv4>",
			applied:  true,
			rules:    []string{"ipv4"},
		},
		{
			name:     "Email address",
			input:    "Contact user@example.com for support",
			expected: "Contact <email> for support",
			applied:  true,
			rules:    []string{"email"},
		},
		{
			name:     "UUID",
			input:    "Session ID: 123e4567-e89b-12d3-a456-426614174000",
			expected: "Session ID: <uuid>",
			applied:  true,
			rules:    []string{"uuid"}, // Only UUID rule should match now
		},
		{
			name:     "Credit card",
			input:    "Payment with card 4111-1111-1111-1111",
			expected: "Payment with card <cc>",
			applied:  true,
			rules:    []string{"cc"},
		},
		{
			name:     "Phone number",
			input:    "Call +1-555-123-4567 for support",
			expected: "Call <phone> for support",
			applied:  true,
			rules:    []string{"phone"},
		},
		{
			name:     "URL",
			input:    "Visit https://example.com for more info",
			expected: "Visit <url> for more info",
			applied:  true,
			rules:    []string{"url"},
		},
		{
			name:     "Multiple PII types",
			input:    "User john@example.com from 192.168.1.1 visited https://example.com",
			expected: "User <email> from <ipv4> visited <url>",
			applied:  true,
			rules:    []string{"email", "ipv4", "url"},
		},
		{
			name:     "No PII",
			input:    "This is a normal log message",
			expected: "This is a normal log message",
			applied:  false,
			rules:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, report := redactor.RedactMessage(tt.input)

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}

			if report.Applied != tt.applied {
				t.Errorf("Expected Applied=%v, got %v", tt.applied, report.Applied)
			}

			if len(report.Rules) != len(tt.rules) {
				t.Errorf("Expected %d rules, got %d", len(tt.rules), len(report.Rules))
			}
		})
	}
}

func TestNormalizeRawLogWithRedaction(t *testing.T) {
	rawLog := RawLog{
		Timestamp: time.Now(),
		Labels:    map[string]string{"service": "test"},
		Payload:   "User john@example.com connected from 192.168.1.1",
	}

	normalized := NormalizeRawLog(rawLog, 1024)

	if normalized.Message != "User <email> connected from <ipv4>" {
		t.Errorf("Expected redacted message, got %q", normalized.Message)
	}

	if !normalized.Sanitized {
		t.Error("Expected Sanitized to be true")
	}

	if !normalized.Redaction.Applied {
		t.Error("Expected Redaction.Applied to be true")
	}

	if len(normalized.Redaction.Rules) != 2 {
		t.Errorf("Expected 2 redaction rules, got %d", len(normalized.Redaction.Rules))
	}
}

func TestLuhnAlgorithm(t *testing.T) {
	tests := []struct {
		number   string
		expected bool
	}{
		{"4111111111111111", true},  // Valid Visa
		{"5555555555554444", true},  // Valid Mastercard
		{"378282246310005", true},   // Valid Amex
		{"1234567890123456", false}, // Invalid
		{"4111111111111112", false}, // Invalid (changed last digit)
		{"", false},                 // Empty
		{"abc", false},              // Non-numeric
	}

	for _, tt := range tests {
		t.Run(tt.number, func(t *testing.T) {
			result := isValidCreditCard(tt.number)
			if result != tt.expected {
				t.Errorf("Expected %v for %s, got %v", tt.expected, tt.number, result)
			}
		})
	}
}
