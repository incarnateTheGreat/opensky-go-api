package main

import (
	"testing"
)

// =============================================================================
// HELPER FUNCTION TESTS
// =============================================================================

func TestSafeString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"valid string", "hello", "hello"},
		{"empty string", "", ""},
		{"nil", nil, ""},
		{"int", 42, ""},
		{"float", 3.14, ""},
		{"bool", true, ""},
		{"slice", []int{1, 2, 3}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeString(tt.input)
			if result != tt.expected {
				t.Errorf("safeString(%v) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeStringPtr(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expectNil bool
		expectVal string
	}{
		{"valid string", "hello", false, "hello"},
		{"empty string", "", true, ""},
		{"nil", nil, true, ""},
		{"int", 42, true, ""},
		{"float", 3.14, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeStringPtr(tt.input)
			if tt.expectNil {
				if result != nil {
					t.Errorf("safeStringPtr(%v) = %v, expected nil", tt.input, *result)
				}
			} else {
				if result == nil {
					t.Errorf("safeStringPtr(%v) = nil, expected %q", tt.input, tt.expectVal)
				} else if *result != tt.expectVal {
					t.Errorf("safeStringPtr(%v) = %q, expected %q", tt.input, *result, tt.expectVal)
				}
			}
		})
	}
}

func TestSafeFloat64Ptr(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expectNil bool
		expectVal float64
	}{
		{"valid float", 3.14, false, 3.14},
		{"zero", 0.0, false, 0.0},
		{"negative", -42.5, false, -42.5},
		{"nil", nil, true, 0},
		{"string", "3.14", true, 0},
		{"int", 42, true, 0}, // Note: JSON decodes as float64, not int
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeFloat64Ptr(tt.input)
			if tt.expectNil {
				if result != nil {
					t.Errorf("safeFloat64Ptr(%v) = %v, expected nil", tt.input, *result)
				}
			} else {
				if result == nil {
					t.Errorf("safeFloat64Ptr(%v) = nil, expected %v", tt.input, tt.expectVal)
				} else if *result != tt.expectVal {
					t.Errorf("safeFloat64Ptr(%v) = %v, expected %v", tt.input, *result, tt.expectVal)
				}
			}
		})
	}
}

func TestSafeInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
	}{
		{"positive float", 42.0, 42},
		{"negative float", -10.0, -10},
		{"float with decimal", 3.7, 3}, // truncates
		{"zero", 0.0, 0},
		{"nil", nil, 0},
		{"string", "42", 0},
		{"bool", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeInt64(tt.input)
			if result != tt.expected {
				t.Errorf("safeInt64(%v) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeBool(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"nil", nil, false},
		{"string true", "true", false},
		{"int 1", 1, false},
		{"int 0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeBool(tt.input)
			if result != tt.expected {
				t.Errorf("safeBool(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
