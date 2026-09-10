package etcdkeys

import (
	"errors"
	"fmt"
	"strings"
)

// BuildNamespacePrefix constructs the etcd key prefix for a given namespace.
// result: <prefix>/<namespace>/
func BuildNamespacePrefix(prefix, namespace string) string {
	return fmt.Sprintf("%s/%s/", prefix, namespace)
}

// BuildExecutorsPrefix constructs the etcd key prefix for executors within a given namespace.
// result: <prefix>/<namespace>/executors/
func BuildExecutorsPrefix(prefix, namespace string) string {
	return fmt.Sprintf("%sexecutors/", BuildNamespacePrefix(prefix, namespace))
}

// BuildExecutorIDPrefix constructs the etcd key prefix for a specific executor within a namespace.
// result: <prefix>/<namespace>/executors/<executorID>/
func BuildExecutorIDPrefix(prefix, namespace, executorID string) string {
	return fmt.Sprintf("%s%s/", BuildExecutorsPrefix(prefix, namespace), executorID)
}

// ExecutorKeyType represents the allowed executor-level key types in etcd.
// Use BuildExecutorKey to construct keys of these types.
type ExecutorKeyType string

const (
	ExecutorHeartbeatKey       ExecutorKeyType = "heartbeat"
	ExecutorStatusKey          ExecutorKeyType = "status"
	ExecutorReportedShardsKey  ExecutorKeyType = "reported_shards"
	ExecutorAssignedStateKey   ExecutorKeyType = "assigned_state"
	ExecutorMetadataKey        ExecutorKeyType = "metadata"
	ExecutorShardStatisticsKey ExecutorKeyType = "statistics"
)

// validExecutorKeyTypes defines the set of valid executor key types.
var validExecutorKeyTypes = map[ExecutorKeyType]struct{}{
	ExecutorHeartbeatKey:       {},
	ExecutorStatusKey:          {},
	ExecutorReportedShardsKey:  {},
	ExecutorAssignedStateKey:   {},
	ExecutorMetadataKey:        {},
	ExecutorShardStatisticsKey: {},
}

// IsValidExecutorKeyType checks if the provided key type is valid.
func IsValidExecutorKeyType(keyType ExecutorKeyType) bool {
	_, exist := validExecutorKeyTypes[keyType]
	return exist
}

// BuildExecutorKey constructs the etcd key for a specific executor and key type.
// result: <prefix>/<namespace>/executors/<executorID>/<keyType>
func BuildExecutorKey(prefix, namespace, executorID string, keyType ExecutorKeyType) string {
	return fmt.Sprintf("%s%s", BuildExecutorIDPrefix(prefix, namespace, executorID), keyType)
}

// ParseExecutorKey parses an etcd key and extracts the executor ID and key type.
// It returns an error if the key does not conform to the expected format.
// Expected format of key: <prefix>/<namespace>/executors/<executorID>/<keyType>
func ParseExecutorKey(prefix, namespace, key string) (executorID string, keyType ExecutorKeyType, err error) {
	prefix = BuildExecutorsPrefix(prefix, namespace)
	if !strings.HasPrefix(key, prefix) {
		return "", "", fmt.Errorf("key '%s' does not have expected prefix '%s'", key, prefix)
	}
	remainder := strings.TrimPrefix(key, prefix)
	parts := strings.Split(remainder, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unexpected key format: %s", key)
	}
	// For metadata keys, the format is: executorID/metadata/metadataKey
	// For other keys, the format is: executorID/keyType
	// We return executorID and the first keyType (e.g., "metadata")
	if len(parts) > 2 && ExecutorKeyType(parts[1]) == ExecutorMetadataKey {
		// This is a metadata key, return "metadata" as the keyType
		return parts[0], ExecutorMetadataKey, nil
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected key format: %s", key)
	}
	if !IsValidExecutorKeyType(ExecutorKeyType(parts[1])) {
		return "", "", fmt.Errorf("invalid executor key type: %s", parts[1])
	}
	return parts[0], ExecutorKeyType(parts[1]), nil
}

// BuildMetadataKey constructs the etcd key for a specific metadata entry of an executor.
// result: <prefix>/<namespace>/executors/<executorID>/metadata/<metadataKey>
func BuildMetadataKey(prefix string, namespace, executorID, metadataKey string) string {
	return fmt.Sprintf("%s/%s", BuildExecutorKey(prefix, namespace, executorID, ExecutorMetadataKey), metadataKey)
}

// BuildDrainedShardsPrefix constructs the etcd key prefix for drained shards within a given namespace.
// Drained shards live in their own keyspace, a sibling of executors/, so draining and undraining a
// shard is a single atomic put or delete.
// Result: <prefix>/<namespace>/drained_shards/
func BuildDrainedShardsPrefix(prefix, namespace string) string {
	return fmt.Sprintf("%sdrained_shards/", BuildNamespacePrefix(prefix, namespace))
}

// BuildDrainedShardKey constructs the etcd key marking a single shard as drained.
// The value stored at this key is empty; the presence of the key is the entire signal.
// Result: <prefix>/<namespace>/drained_shards/<shardID>
func BuildDrainedShardKey(prefix, namespace, shardID string) string {
	return fmt.Sprintf("%s%s", BuildDrainedShardsPrefix(prefix, namespace), shardID)
}

// ValidateShardID rejects shard IDs that cannot survive a key round trip
func ValidateShardID(shardID string) error {
	if shardID == "" {
		return errors.New("shard ID must not be empty")
	}
	if strings.Contains(shardID, "/") {
		return fmt.Errorf("shard ID '%s' must not contain '/'", shardID)
	}
	return nil
}

// ParseDrainedShardKey extracts the shard ID from a drained-shard etcd key.
// Expected format: <prefix>/<namespace>/drained_shards/<shardID>
func ParseDrainedShardKey(prefix, namespace, key string) (shardID string, err error) {
	drainedPrefix := BuildDrainedShardsPrefix(prefix, namespace)
	if !strings.HasPrefix(key, drainedPrefix) {
		return "", fmt.Errorf("key '%s' does not have expected drained shards prefix '%s'", key, drainedPrefix)
	}
	shardID = strings.TrimPrefix(key, drainedPrefix)
	if err := ValidateShardID(shardID); err != nil {
		return "", fmt.Errorf("unexpected drained shard key format '%s': %w", key, err)
	}
	return shardID, nil
}

const maxHostnameLength = 128

// BuildDrainedHostsPrefix constructs the etcd key prefix for drained hosts within a given namespace
// Expected format: <prefix>/<namespace>/drained_hosts/
func BuildDrainedHostsPrefix(prefix, namespace string) string {
	return fmt.Sprintf("%sdrained_hosts/", BuildNamespacePrefix(prefix, namespace))
}

// BuildDrainedHostKey constructs the etcd key marking a single host as drained
// Expected format: <prefix>/<namespace>/drained_hosts/<hostname>
func BuildDrainedHostKey(prefix, namespace, hostname string) string {
	return fmt.Sprintf("%s%s", BuildDrainedHostsPrefix(prefix, namespace), hostname)
}

// ValidateHostname rejects hostnames that cannot survive a key round trip or that
// would break hostname@uuid matching.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return errors.New("hostname must not be empty")
	}
	if len(hostname) > maxHostnameLength {
		return fmt.Errorf("hostname exceeds %d bytes", maxHostnameLength)
	}
	if strings.Contains(hostname, "/") {
		return fmt.Errorf("hostname '%s' must not contain '/'", hostname)
	}
	if strings.Contains(hostname, "@") {
		return fmt.Errorf("hostname '%s' must not contain '@'", hostname)
	}
	return nil
}

// ParseDrainedHostKey extracts the hostname from a drained-host etcd key.
// Expected format: <prefix>/<namespace>/drained_hosts/<hostname>
func ParseDrainedHostKey(prefix, namespace, key string) (hostname string, err error) {
	drainedPrefix := BuildDrainedHostsPrefix(prefix, namespace)
	if !strings.HasPrefix(key, drainedPrefix) {
		return "", fmt.Errorf("key '%s' does not have expected drained hosts prefix '%s'", key, drainedPrefix)
	}
	hostname = strings.TrimPrefix(key, drainedPrefix)
	if err := ValidateHostname(hostname); err != nil {
		return "", fmt.Errorf("unexpected drained host key format '%s': %w", key, err)
	}
	return hostname, nil
}
