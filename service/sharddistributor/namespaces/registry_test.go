package namespaces

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

func TestNewRegistryValidation(t *testing.T) {
	tests := []struct {
		name     string
		cfg      []config.Namespace
		wantList []Namespace
		wantErr  string
	}{
		{
			name: "valid",
			cfg: []config.Namespace{
				{Name: "fixed", Type: config.NamespaceTypeFixed, ShardNum: 4},
				{Name: "ephemeral", Type: config.NamespaceTypeEphemeral},
			},
			wantList: []Namespace{
				{Name: "fixed", Type: TypeFixed, ShardNum: 4},
				{Name: "ephemeral", Type: TypeEphemeral},
			},
		},
		{
			name:    "empty name",
			cfg:     []config.Namespace{{Type: config.NamespaceTypeEphemeral}},
			wantErr: `namespace "": name must not be empty`,
		},
		{
			name:    "unknown type",
			cfg:     []config.Namespace{{Name: "ns", Type: "ephmeral"}},
			wantErr: `namespace "ns": unknown type "ephmeral"`,
		},
		{
			name:    "fixed without shards",
			cfg:     []config.Namespace{{Name: "ns", Type: config.NamespaceTypeFixed}},
			wantErr: `namespace "ns": fixed namespace must have a positive shardNum, got 0`,
		},
		{
			name: "duplicate name",
			cfg: []config.Namespace{
				{Name: "ns", Type: config.NamespaceTypeEphemeral},
				{Name: "ns", Type: config.NamespaceTypeFixed, ShardNum: 1},
			},
			wantErr: `namespace "ns": configured more than once`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRegistry(config.ShardDistribution{Namespaces: tt.cfg})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, r)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantList, r.List())
		})
	}
}

func TestRegistryGet(t *testing.T) {
	r := MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{
		{Name: "fixed", Type: config.NamespaceTypeFixed, ShardNum: 4},
		{Name: "ephemeral", Type: config.NamespaceTypeEphemeral},
	}})

	tests := []struct {
		name    string
		lookup  string
		wantNS  Namespace
		wantErr error
	}{
		{name: "found", lookup: "ephemeral", wantNS: Namespace{Name: "ephemeral", Type: TypeEphemeral}},
		{name: "not found", lookup: "unknown", wantErr: &types.NamespaceNotFoundError{Namespace: "unknown"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := r.Get(tt.lookup)
			require.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantNS, ns)
		})
	}
}

func TestRegistryListIsACopy(t *testing.T) {
	r := MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{{Name: "a", Type: config.NamespaceTypeEphemeral}}})

	r.List()[0].Name = "mutated"

	assert.Equal(t, "a", r.List()[0].Name)
}

func TestMustNewRegistryPanicsOnInvalidConfig(t *testing.T) {
	assert.Panics(t, func() {
		MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{{Name: "ns"}}})
	})
}
