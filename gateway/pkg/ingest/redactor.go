package ingest

import (
	"regexp"
	"strconv"
	"strings"
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

			// IPv6 addresses (simplified pattern)
			"ipv6": regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b|\b::1\b|\b::\b`),

			// Email addresses
			"email": regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),

			// UUIDs (v1, v4, v5)
			"uuid": regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),

			// Credit card numbers (13-19 digits, with optional separators)
			"cc": regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{1,4}\b`),

			// Phone numbers (E.164 format and common formats)
			"phone": regexp.MustCompile(`\+1?[-.\s]?\(?[0-9]{3}\)?[-.\s]?[0-9]{3}[-.\s]?[0-9]{4}\b|\(?[0-9]{3}\)?[-.\s]?[0-9]{3}[-.\s]?[0-9]{4}(?:\s|$|\b)|\+\d{1,3}\d{4,14}\b`),

			// URLs (http/https)
			"url": regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`),
		},
	}
}

// RedactMessage applies PII redaction to a message and returns the redacted message and report
func (r *PIIRedactor) RedactMessage(message string) (string, RedactionReport) {
	report := RedactionReport{
		Applied: false,
		Rules:   []string{},
		Count:   0,
	}

	redactedMessage := message

	// Process rules in a specific order to avoid conflicts
	// Process more specific patterns first (UUID, credit card) before general ones (phone)
	ruleOrder := []string{"uuid", "cc", "email", "ipv4", "ipv6", "url", "phone"}

	for _, ruleName := range ruleOrder {
		regex, exists := r.rules[ruleName]
		if !exists {
			continue
		}

		matches := regex.FindAllString(redactedMessage, -1)
		if len(matches) > 0 {
			// Special handling for credit card numbers - validate with Luhn algorithm
			if ruleName == "cc" {
				validMatches := []string{}
				for _, match := range matches {
					// Clean the match (remove separators)
					cleanMatch := strings.ReplaceAll(strings.ReplaceAll(match, "-", ""), " ", "")
					if isValidCreditCard(cleanMatch) {
						validMatches = append(validMatches, match)
					}
				}
				if len(validMatches) > 0 {
					report.Rules = append(report.Rules, ruleName)
					report.Count += len(validMatches)
					report.Applied = true
					redactedMessage = regex.ReplaceAllString(redactedMessage, "<cc>")
				}
			} else {
				report.Rules = append(report.Rules, ruleName)
				report.Count += len(matches)
				report.Applied = true
				redactedMessage = regex.ReplaceAllString(redactedMessage, "<"+ruleName+">")
			}
		}
	}

	return redactedMessage, report
}

// isValidCreditCard validates a credit card number using the Luhn algorithm
func isValidCreditCard(number string) bool {
	// Remove any non-digit characters
	cleanNumber := strings.ReplaceAll(strings.ReplaceAll(number, "-", ""), " ", "")

	// Check if it's all digits and has reasonable length
	if len(cleanNumber) < 13 || len(cleanNumber) > 19 {
		return false
	}

	// Check if all characters are digits
	for _, char := range cleanNumber {
		if char < '0' || char > '9' {
			return false
		}
	}

	// Apply Luhn algorithm
	sum := 0
	alternate := false

	// Process digits from right to left
	for i := len(cleanNumber) - 1; i >= 0; i-- {
		digit, _ := strconv.Atoi(string(cleanNumber[i]))

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
