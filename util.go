package main

import (
	"regexp"
	"strings"
)

var (
	pattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

func propertyKey(s string) string {
	// Add underscore before uppercase letters that follow lowercase letters
	s = pattern.ReplaceAllString(s, "_")
	return strings.ToLower(s)
}
