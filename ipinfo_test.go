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
