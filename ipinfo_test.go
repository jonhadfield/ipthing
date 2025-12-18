package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShouldUpdateIPInfo(t *testing.T) {
	tests := []struct {
		name         string
		lastUpdate   time.Time
		shouldUpdate bool
	}{
		{
			name:         "Recent update - within 24 hours",
			lastUpdate:   time.Now().Add(-12 * time.Hour),
			shouldUpdate: false,
		},
		{
			name:         "Old update - over 24 hours",
			lastUpdate:   time.Now().Add(-25 * time.Hour),
			shouldUpdate: true,
		},
		{
			name:         "Very old update - over 7 days",
			lastUpdate:   time.Now().Add(-8 * 24 * time.Hour),
			shouldUpdate: true,
		},
		{
			name:         "Zero time - never updated",
			lastUpdate:   time.Time{},
			shouldUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldUpdateIPInfo(tt.lastUpdate)
			assert.Equal(t, tt.shouldUpdate, result)
		})
	}
}

func TestStripPrivateIPs(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "203.0.113.1,192.168.1.1,::1",
			expected: "203.0.113.1,192.168.1.1",
		},
		{
			input:    "203.0.113.1",
			expected: "203.0.113.1",
		},
		{
			input:    "::1",
			expected: "",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		result := stripPrivateIPs(tt.input)
		assert.Equal(t, tt.expected, result, "Input %s should return %s", tt.input, tt.expected)
	}
}
