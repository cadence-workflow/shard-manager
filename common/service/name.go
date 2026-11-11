package service

import "strings"

const (
	_servicePrefix = "cadence-"

	// ShardDistributor is the name of the shard distributor service
	ShardDistributor = "shard-distributor"
)

// List contains the list of all supported services for shard-manager
var List = []string{ShardDistributor}

// ListWithRing contains services that use a hash ring (none for shard-manager)
var ListWithRing = []string{}

// ShortName returns cadence service name without "cadence-" prefix
func ShortName(name string) string {
	return strings.TrimPrefix(name, _servicePrefix)
}

// FullName returns cadence service name with "cadence-" prefix
func FullName(name string) string {
	if strings.HasPrefix(name, _servicePrefix) {
		return name
	}
	return _servicePrefix + name
}

func ShortNames(names []string) []string {
	result := make([]string, len(names))
	for i := range names {
		result[i] = ShortName(names[i])
	}
	return result
}
