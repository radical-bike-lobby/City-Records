package main

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	pattern   = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	maxLength = 124
)

func cleanText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1 // Drop non-printable characters
	}, text)
}

// takeRunes returns up to bytesCount bytes worth of runes from text
// UTF-8 chars can be 32bits, so this ensures we don't end up with non-printable chars
func takeRunes(text string, byteCount int) (result string) {
	var b strings.Builder
	for _, rune := range text {
		byteCount -= utf8.RuneLen(rune)
		if byteCount < 0 {
			break
		}
		b.WriteRune(rune)
	}
	return b.String()
}

// propertyKey formats the string so it is suitable as a google drive property key
// all non-alphanumerics are replace with underscore and the string length is limited
// to 124 characters
// Returns a lower cased key, and same-cased value
func propertyKeyValue(key, value string) (string, string) {
	key = strings.ToLower(cleanText(key))
	value = cleanText(value)
	// Replace non ascii chars with '_'
	key = pattern.ReplaceAllString(key, "_")
	key = takeRunes(key, maxLength)

	remaining := maxLength - len(key)
	value = takeRunes(value, remaining)

	return key, value
}
