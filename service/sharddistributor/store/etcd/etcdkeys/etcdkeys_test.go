package etcdkeys

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildNamespacePrefix(t *testing.T) {
	got := BuildNamespacePrefix("/cadence", "test-ns")
	assert.Equal(t, "/cadence/test-ns", got)
}

func TestBuildExecutorsPrefix(t *testing.T) {
	got := BuildExecutorsPrefix("/cadence", "test-ns")
	assert.Equal(t, "/cadence/test-ns/executors/", got)
}

func TestBuildExecutorKey(t *testing.T) {
	got := BuildExecutorKey("/cadence", "test-ns", "exec-1", "heartbeat")
	assert.Equal(t, "/cadence/test-ns/executors/exec-1/heartbeat", got)
}

func TestParseExecutorKey(t *testing.T) {
	// Valid key
	executorID, keyType, err := ParseExecutorKey("/cadence", "test-ns", "/cadence/test-ns/executors/exec-1/heartbeat")
	assert.NoError(t, err)
	assert.Equal(t, "exec-1", executorID)
	assert.Equal(t, ExecutorHeartbeatKey, keyType)

	// Prefix missing
	_, _, err = ParseExecutorKey("/cadence", "test-ns", "/wrong/prefix")
	assert.ErrorContains(t, err, "key '/wrong/prefix' does not have expected prefix '/cadence/test-ns/executors/'")

	// Unexpected key format
	_, _, err = ParseExecutorKey("/cadence", "test-ns", "/cadence/test-ns/executors/exec-1/heartbeat/extra")
	assert.ErrorContains(t, err, "unexpected key format: /cadence/test-ns/executors/exec-1/heartbeat/extra")
}

func TestBuildMetadataKey(t *testing.T) {
	got := BuildMetadataKey("/cadence", "test-ns", "exec-1", "my-metadata-key")
	assert.Equal(t, "/cadence/test-ns/executors/exec-1/metadata/my-metadata-key", got)
}

func TestParseExecutorKey_MetadataKey(t *testing.T) {
	// Test that ParseExecutorKey correctly identifies metadata keys
	// and that we can extract the metadata key name from the full key
	metadataKey := BuildMetadataKey("/cadence", "test-ns", "exec-1", "hostname")

	executorID, keyType, err := ParseExecutorKey("/cadence", "test-ns", metadataKey)
	assert.NoError(t, err)
	assert.Equal(t, "exec-1", executorID)
	assert.Equal(t, ExecutorMetadataKey, keyType)
}

func TestParseExecutorKey_InvalidKeyType(t *testing.T) {
	key := BuildExecutorIDPrefix("/cadence", "test-ns", "exec-1") + "invalid_type"
	_, _, err := ParseExecutorKey("/cadence", "test-ns", key)
	assert.ErrorContains(t, err, "invalid executor key type: invalid_type")
}

func TestBuildDrainedShardsPrefix(t *testing.T) {
	got := BuildDrainedShardsPrefix("/cadence", "test-ns")
	assert.Equal(t, "/cadence/test-ns/drained_shards/", got)
}

func TestBuildDrainedShardKey(t *testing.T) {
	got := BuildDrainedShardKey("/cadence", "test-ns", "shard-1")
	assert.Equal(t, "/cadence/test-ns/drained_shards/shard-1", got)
}

// The drained keyspace must not collide with the executor keyspace, otherwise a
// prefix scan for one would pick up keys belonging to the other.
func TestDrainedShardsPrefixIsDisjointFromExecutorsPrefix(t *testing.T) {
	drained := BuildDrainedShardsPrefix("/cadence", "test-ns")
	executors := BuildExecutorsPrefix("/cadence", "test-ns")
	assert.NotEqual(t, drained, executors)
	assert.False(t, strings.HasPrefix(drained, executors))
	assert.False(t, strings.HasPrefix(executors, drained))
}

func TestParseDrainedShardKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		wantShardID string
		wantErr     string
	}{
		{
			name:        "valid",
			key:         "/cadence/test-ns/drained_shards/shard-42",
			wantShardID: "shard-42",
		},
		{
			name:    "wrong prefix",
			key:     "/wrong/prefix/drained_shards/shard-42",
			wantErr: "does not have expected drained shards prefix",
		},
		{
			name:    "different namespace",
			key:     "/cadence/other-ns/drained_shards/shard-42",
			wantErr: "does not have expected drained shards prefix",
		},
		{
			name:    "empty shard id",
			key:     "/cadence/test-ns/drained_shards/",
			wantErr: "unexpected drained shard key format",
		},
		{
			name:    "extra path segment",
			key:     "/cadence/test-ns/drained_shards/shard-42/extra",
			wantErr: "unexpected drained shard key format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			shardID, err := ParseDrainedShardKey("/cadence", "test-ns", tc.key)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, shardID)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantShardID, shardID)
		})
	}
}

func TestDrainedShardKeyRoundTrip(t *testing.T) {
	for _, shardID := range []string{"0", "31", "shard-1", "fixed-32", "abc.def", "UPPER_case-9"} {
		t.Run(shardID, func(t *testing.T) {
			require.NoError(t, ValidateShardID(shardID))

			key := BuildDrainedShardKey("/cadence", "test-ns", shardID)
			got, err := ParseDrainedShardKey("/cadence", "test-ns", key)
			assert.NoError(t, err)
			assert.Equal(t, shardID, got)
		})
	}
}

func TestValidateShardID(t *testing.T) {
	tests := []struct {
		name    string
		shardID string
		wantErr string
	}{
		{name: "simple", shardID: "shard-42"},
		{name: "numeric", shardID: "0"},
		{name: "dots and underscores", shardID: "abc.def_9"},
		{name: "empty", shardID: "", wantErr: "must not be empty"},
		{name: "leading slash", shardID: "/shard-42", wantErr: "must not contain '/'"},
		{name: "trailing slash", shardID: "shard-42/", wantErr: "must not contain '/'"},
		{name: "embedded slash", shardID: "shard/42", wantErr: "must not contain '/'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateShardID(tc.shardID)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// ValidateShardID is the rule ParseDrainedShardKey enforces on read, so anything it
// accepts must survive a key round trip and anything it rejects must be refused.
func TestValidateShardIDMatchesKeyParsing(t *testing.T) {
	for _, shardID := range []string{"shard-42", "0", "abc.def_9", "", "/leading", "trailing/", "a/b"} {
		t.Run(shardID, func(t *testing.T) {
			key := BuildDrainedShardKey("/cadence", "test-ns", shardID)
			_, parseErr := ParseDrainedShardKey("/cadence", "test-ns", key)

			if ValidateShardID(shardID) != nil {
				assert.Error(t, parseErr, "rejected shard ID must not parse back")
				return
			}
			assert.NoError(t, parseErr, "accepted shard ID must parse back")
		})
	}
}
