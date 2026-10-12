package namespaces

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespace(t *testing.T) {
	tests := []struct {
		name            string
		ns              Namespace
		wantType        string
		wantEphemeral   bool
		wantFixedShards []string
	}{
		{
			name:            "fixed",
			ns:              Namespace{Name: "ns", Type: TypeFixed, ShardNum: 3},
			wantType:        "fixed",
			wantFixedShards: []string{"0", "1", "2"},
		},
		{
			name:          "ephemeral",
			ns:            Namespace{Name: "ns", Type: TypeEphemeral},
			wantType:      "ephemeral",
			wantEphemeral: true,
		},
		{
			name:     "zero value",
			ns:       Namespace{},
			wantType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantType, tt.ns.Type.String())
			assert.Equal(t, tt.wantEphemeral, tt.ns.IsEphemeral())
			assert.Equal(t, tt.wantFixedShards, tt.ns.FixedShards())
		})
	}
}
