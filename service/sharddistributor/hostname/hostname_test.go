package hostname

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "executor-1", want: "executor-1"},
		{name: "slashes are replaced", raw: "executor/1", want: "executor_1"},
		{name: "at signs are replaced", raw: "host@name", want: "host_name"},
		{name: "truncated", raw: strings.Repeat("a", MaxLength+1), want: strings.Repeat("a", MaxLength)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.raw)
			assert.Equal(t, tt.want, got)
			if got != "" {
				require.NoError(t, Validate(got))
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  string
	}{
		{name: "plain", hostname: "host-a"},
		{name: "empty", hostname: "", wantErr: "must not be empty"},
		{name: "at sign", hostname: "host@name", wantErr: "must not contain '@'"},
		{name: "slash", hostname: "host/name", wantErr: "must not contain '/'"},
		{name: "max length", hostname: strings.Repeat("a", MaxLength)},
		{name: "too long", hostname: strings.Repeat("a", MaxLength+1), wantErr: "exceeds 128 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.hostname)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
