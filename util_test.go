package main

import (
	"strings"
	"testing"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty string", "", ""},
		{"Basic printable characters", "Hello World 123", "Hello World 123"},
		{"Non-printable characters", "Hello\x00World\t\n", "HelloWorld"},
		{"Unicode characters", "你好世界", "你好世界"},
		{"Mixed printable and non-printable", "Test\u200bString\ufeffwith\nInvisible\x01Chars", "TestStringwithInvisibleChars"},
		{"Newline characters", "Line1\nLine2", "Line1Line2"},
		{"Space characters", "Word1 Word2", "Word1 Word2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanText(tt.input)
			if result != tt.expected {
				t.Errorf("For input %q: expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestPropertyKeyValue(t *testing.T) {
	tests := []struct {
		name          string
		keyInput      string
		valueInput    string
		expectedKey   string
		expectedValue string
	}{
		{"Basic key and value", "MyProperty", "Some Value", "myproperty", "Some Value"},
		{"Key with spaces and special chars", "My Property Key!", "Value", "my_property_key_", "Value"},
		{"Value with spaces and special chars", "Key", "A Value With Spaces & Symbols", "key", "A Value With Spaces & Symbols"},
		{"Key exceeding maxLength", strings.Repeat("a", 150), "Value", strings.Repeat("a", maxLength), ""},
		{"Value exceeding remainingLength", "short_key", strings.Repeat("b", 150), "short_key", strings.Repeat("b", maxLength-len("short_key"))},
		{"Both exceeding maxLength", strings.Repeat("k", 130), strings.Repeat("v", 130), strings.Repeat("k", maxLength), ""}, // Value should be truncated to 0 in this case
		{"Empty key and value", "", "", "", ""},
		{"Key with non-printable", "key\x00_test", "value", "key_test", "value"},
		{"Value with non-printable", "key", "value\x01_test", "key", "value_test"},
		{"Value with smart quotes", "key", "“value\x01_test”", "key", "“value_test”"},
		{"Mixed case key", "MixedCaseKey", "value", "mixedcasekey", "value"},
		{"Mixed case value", "key", "MixedCaseValue", "key", "MixedCaseValue"},
		{"Key length near maxLength, value large", strings.Repeat("k", maxLength-10), strings.Repeat("v", 20), strings.Repeat("k", maxLength-10), strings.Repeat("v", 10)},
		{"Smart quotes", "name", "Accept and Acknowledge Report from the Berkeley Police Review Commission, “To Achieve Fairness and Impartiality,” and Refer Key Recommendations to the City Manager for Policy Development and Consideration in September 2018 Report to City Council", "name", "Accept and Acknowledge Report from the Berkeley Police Review Commission, “To Achieve Fairness and Impartiality,” an"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualKey, actualValue := propertyKeyValue(tt.keyInput, tt.valueInput)
			if actualKey != tt.expectedKey {
				t.Errorf("For keyInput %q, valueInput %q: expected key %q, got %q", tt.keyInput, tt.valueInput, tt.expectedKey, actualKey)
			}
			if actualValue != tt.expectedValue {
				t.Errorf("For keyInput %q, valueInput %q: expected value %q, got %q", tt.keyInput, tt.valueInput, tt.expectedValue, actualValue)
			}
		})
	}
}
