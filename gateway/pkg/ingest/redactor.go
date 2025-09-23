package ingest

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
)

// PIIRedactor handles PII redaction using compiled regexes
type PIIRedactor struct {
	rules map[string]*regexp.Regexp
}

// NewPIIRedactor creates a new PII redactor with compiled regexes
func NewPIIRedactor() *PIIRedactor {
	return &PIIRedactor{
		rules: map[string]*regexp.Regexp{
			// IPv4 addresses (including private ranges)
			"ipv4": regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),

			// IPv6 addresses
			"ipv6": regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`),

			// Email addresses
			"email": regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),

			// UUIDs (v4 format)
			"uuid": regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}\b`),

			// Credit card numbers (with Luhn check)
			"cc": regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),

			// Phone numbers (E.164 format)
			"phone": regexp.MustCompile(`\b\+?[1-9]\d{1,14}\b`),

			// URLs
			"url": regexp.MustCompile(`https?://[^\s]+`),
		},
	}
}

// RedactMessage applies PII redaction to a message and returns the redacted message and report
func (r *PIIRedactor) RedactMessage(message string) (string, logtypes.RedactionReport) {
	report := logtypes.RedactionReport{
		Applied: false,
		Rules:   []string{},
		Count:   0,
	}

	redactedMessage := message

	// Apply redaction rules in order (most specific first)
	ruleOrder := []string{"uuid", "ipv6", "ipv4", "email", "cc", "phone", "url"}

	for _, ruleName := range ruleOrder {
		rule := r.rules[ruleName]
		matches := rule.FindAllString(redactedMessage, -1)

		if len(matches) > 0 {
			// Special handling for credit card numbers (Luhn check)
			if ruleName == "cc" {
				validMatches := []string{}
				for _, match := range matches {
					if isValidCreditCard(match) {
						validMatches = append(validMatches, match)
					}
				}
				matches = validMatches
			}

			if len(matches) > 0 {
				report.Applied = true
				report.Rules = append(report.Rules, ruleName)
				report.Count += len(matches)

				// Replace matches with placeholder
				placeholder := "<" + ruleName + ">"
				for _, match := range matches {
					redactedMessage = strings.ReplaceAll(redactedMessage, match, placeholder)
				}
			}
		}
	}

	return redactedMessage, report
}

// isValidCreditCard checks if a credit card number passes the Luhn algorithm
func isValidCreditCard(cc string) bool {
	// Remove spaces and dashes
	cc = regexp.MustCompile(`[-\s]`).ReplaceAllString(cc, "")

	// Check if it's all digits and has valid length
	if !regexp.MustCompile(`^\d{13,19}$`).MatchString(cc) {
		return false
	}

	// Luhn algorithm
	sum := 0
	alternate := false

	// Process digits from right to left
	for i := len(cc) - 1; i >= 0; i-- {
		digit, _ := strconv.Atoi(string(cc[i]))

		if alternate {
			digit *= 2
			if digit > 9 {
				digit = (digit % 10) + 1
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}
