package namespace

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

func TestRegistry_Get(t *testing.T) {
	namespaces := []config.Namespace{
		{Name: "ns-a", Type: config.NamespaceTypeFixed, ShardNum: 8},
		{Name: "ns-b", Type: config.NamespaceTypeEphemeral},
	}
	reg := NewRegistry(namespaces)

	tests := []struct {
		name      string
		query     string
		wantFound bool
		wantNs    config.Namespace
	}{
		{
			name:      "fixed namespace",
			query:     "ns-a",
			wantFound: true,
			wantNs:    namespaces[0],
		},
		{
			name:      "ephemeral namespace",
			query:     "ns-b",
			wantFound: true,
			wantNs:    namespaces[1],
		},
		{
			name:      "unknown namespace",
			query:     "ns-missing",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, ok := reg.Get(tt.query)
			assert.Equal(t, tt.wantFound, ok)
			if tt.wantFound {
				assert.Equal(t, tt.wantNs, ns)
			}
		})
	}
}

func TestRegistry_All(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []config.Namespace
	}{
		{
			name:       "preserves order",
			namespaces: []config.Namespace{{Name: "first"}, {Name: "second"}, {Name: "third"}},
		},
		{
			name:       "nil input",
			namespaces: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(tt.namespaces)
			assert.Equal(t, tt.namespaces, reg.All())
		})
	}
}
