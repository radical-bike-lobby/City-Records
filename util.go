package main

import (
	"regexp"
	"strings"
)

var (
	pattern   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	maxLength = 124
)

// propertyKey formats the string so it is suitable as a google drive property key
// all non-alphanumerics are replace with underscore and the string length is limited
// to 124 characters
func propertyKeyValue(key, value string) (string, string) {
	// Add underscore before uppercase letters that follow lowercase letters
	key = pattern.ReplaceAllString(key, "_")
	if len(key) > maxLength {
		key = key[:maxLength]
	}

	remaining := maxLength - len(key)
	if len(value) > remaining {
		value = value[:remaining]
	}
	return strings.ToLower(key), strings.ToLower(value)
}
