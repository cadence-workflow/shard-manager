package config

import "errors"

// ClusterGroupMetadata identifies the current cluster for dynamic config filters.
type ClusterGroupMetadata struct {
	CurrentClusterName string `yaml:"currentClusterName"`
}

// Validate validates ClusterGroupMetadata.
func (m *ClusterGroupMetadata) Validate() error {
	if m == nil {
		return errors.New("ClusterGroupMetadata cannot be empty")
	}
	if m.CurrentClusterName == "" {
		return errors.New("current cluster name is empty")
	}
	return nil
}
