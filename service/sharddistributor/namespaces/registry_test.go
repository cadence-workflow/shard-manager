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
		name    string
		cfg     []config.Namespace
		wantErr string
	}{
		{
			name: "valid",
			cfg: []config.Namespace{
				{Name: "ns-a", Type: config.NamespaceTypeFixed, ShardNum: 4},
				{Name: "ns-b", Type: config.NamespaceTypeFixed, ShardNum: 8},
			},
		},
		{
			name:    "empty name",
			cfg:     []config.Namespace{{Type: config.NamespaceTypeFixed, ShardNum: 4}},
			wantErr: `namespace "": name must not be empty`,
		},
		{
			name:    "leading underscore",
			cfg:     []config.Namespace{{Name: "_assignment_versions", Type: config.NamespaceTypeFixed, ShardNum: 4}},
			wantErr: `namespace "_assignment_versions": name must not start with an underscore`,
		},
		{
			name:    "unknown type",
			cfg:     []config.Namespace{{Name: "ns", Type: "ephemeral"}},
			wantErr: `namespace "ns": unknown type "ephemeral", must be "fixed"`,
		},
		{
			name:    "without shards",
			cfg:     []config.Namespace{{Name: "ns", Type: config.NamespaceTypeFixed}},
			wantErr: `namespace "ns": shardNum must be positive, got 0`,
		},
		{
			name: "duplicate name",
			cfg: []config.Namespace{
				{Name: "ns", Type: config.NamespaceTypeFixed, ShardNum: 4},
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
			assert.ElementsMatch(t, tt.cfg, r.List())
		})
	}
}

func TestRegistryGet(t *testing.T) {
	nsA := config.Namespace{Name: "ns-a", Type: config.NamespaceTypeFixed, ShardNum: 4}
	nsB := config.Namespace{Name: "ns-b", Type: config.NamespaceTypeFixed, ShardNum: 8}
	r := MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{nsA, nsB}})

	tests := []struct {
		name    string
		lookup  string
		wantNS  config.Namespace
		wantErr error
	}{
		{name: "found", lookup: "ns-b", wantNS: nsB},
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

func TestRegistryList(t *testing.T) {
	b := config.Namespace{Name: "b", Type: config.NamespaceTypeFixed, ShardNum: 2}
	a := config.Namespace{Name: "a", Type: config.NamespaceTypeFixed, ShardNum: 1}
	r := MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{b, a}})

	assert.Equal(t, []config.Namespace{a, b}, r.List(), "sorted by name")

	r.List()[0].Name = "mutated"
	assert.Equal(t, "a", r.List()[0].Name, "returns a copy")
}

func TestMustNewRegistryPanicsOnInvalidConfig(t *testing.T) {
	assert.Panics(t, func() {
		MustNewRegistry(config.ShardDistribution{Namespaces: []config.Namespace{{Name: "ns"}}})
	})
}
