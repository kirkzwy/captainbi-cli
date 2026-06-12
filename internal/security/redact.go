package security

import (
	"regexp"
	"strings"
)

var sensitiveKeys = []string{
	"authorization",
	"access_token",
	"refresh_token",
	"client_secret",
	"secret",
	"token",
}

// RedactValue masks sensitive values while keeping enough shape for debugging.
func RedactValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "********" + value[len(value)-4:]
}

// RedactKeyValue masks values whose key looks sensitive.
func RedactKeyValue(key string, value any) any {
	lower := strings.ToLower(key)
	for _, k := range sensitiveKeys {
		if strings.Contains(lower, k) {
			return RedactValue(toString(value))
		}
	}
	return value
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(access_token["'=:\s]+)[^"',\s}]+`),
	regexp.MustCompile(`(?i)(client_secret["'=:\s]+)[^"',\s}]+`),
	regexp.MustCompile(`(?i)(authorization["'=:\s]+)[^"',\s}]+`),
}

func RedactString(s string) string {
	for _, re := range sensitivePatterns {
		s = re.ReplaceAllString(s, "${1}********")
	}
	return s
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
