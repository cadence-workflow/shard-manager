package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterGroupMetadataValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ClusterGroupMetadata
		err  string
	}{
		{name: "valid", cfg: &ClusterGroupMetadata{CurrentClusterName: "active"}},
		{name: "missing metadata", err: "ClusterGroupMetadata cannot be empty"},
		{name: "missing current cluster", cfg: &ClusterGroupMetadata{}, err: "current cluster name is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.err)
			}
		})
	}
}
