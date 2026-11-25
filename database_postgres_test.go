package main

import (
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPostgresInsufficientPrivilege(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "unrelated", err: errors.New("connection refused"), want: false},
		{name: "pq 42501", err: &pq.Error{Code: "42501", Message: "permission denied"}, want: true},
		{name: "pq other code", err: &pq.Error{Code: "23505", Message: "duplicate"}, want: false},
		{name: "must be owner text", err: errors.New(`must be owner of table http_requests`), want: true},
		{name: "permission denied text", err: errors.New("permission denied for table ip_info"), want: true},
		{
			name: "wrapped pq 42501",
			err:  errors.Join(errors.New("migrate"), &pq.Error{Code: "42501"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isPostgresInsufficientPrivilege(tt.err))
		})
	}
}

func TestHandleSchemaDDLError_NonPrivilege(t *testing.T) {
	p := NewPostgresDB("postgres://unused")
	err := p.handleSchemaDDLError(errors.New("syntax error"), "ip_info")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create ip_info table")
	assert.Contains(t, err.Error(), "syntax error")
	assert.False(t, isPostgresInsufficientPrivilege(err))
}
