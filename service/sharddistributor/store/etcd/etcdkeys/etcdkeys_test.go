package etcdkeys

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildNamespacePrefix(t *testing.T) {
	got := BuildNamespacePrefix("/cadence", "test-ns")
	assert.Equal(t, "/cadence/test-ns/", got)
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

func TestParseExecutorKey_UnknownKeyType(t *testing.T) {
	key := BuildExecutorIDPrefix("/cadence", "test-ns", "exec-1") + "future_field"
	executorID, keyType, err := ParseExecutorKey("/cadence", "test-ns", key)
	assert.NoError(t, err)
	assert.Equal(t, "exec-1", executorID)
	assert.Equal(t, ExecutorKeyType("future_field"), keyType)
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

func TestDrainedHostsPrefixIsDisjointFromSiblingPrefixes(t *testing.T) {
	hosts := BuildDrainedHostsPrefix("/cadence", "test-ns")
	executors := BuildExecutorsPrefix("/cadence", "test-ns")
	shards := BuildDrainedShardsPrefix("/cadence", "test-ns")

	for _, other := range []string{executors, shards} {
		assert.NotEqual(t, hosts, other)
		assert.False(t, strings.HasPrefix(hosts, other))
		assert.False(t, strings.HasPrefix(other, hosts))
	}
}

func TestParseExecutorKey_HostMetadata(t *testing.T) {
	hostMetadataKey := BuildExecutorKey("/cadence", "test-ns", "exec-1", ExecutorHostMetadataKey)
	executorID, keyType, err := ParseExecutorKey("/cadence", "test-ns", hostMetadataKey)
	assert.NoError(t, err)
	assert.Equal(t, "exec-1", executorID)
	assert.Equal(t, ExecutorHostMetadataKey, keyType)
}

func TestBuildRangesPrefix(t *testing.T) {
	got := BuildRangesPrefix("/cadence", "test-ns")
	assert.Equal(t, "/cadence/test-ns/ranges/", got)
}

func TestBuildRangeKey(t *testing.T) {
	got := BuildRangeKey("/cadence", "test-ns", []byte{0x83, 0xee, 0x86, 0x1a, 0xac, 0x65, 0x53, 0x62})
	assert.Equal(t, "/cadence/test-ns/ranges/83ee861aac655362", got)
}

// The ranges keyspace must not collide with the executor, drained-shard, or
// drained-host keyspaces, otherwise a prefix scan for one would pick up keys
// belonging to another.
func TestRangesPrefixIsDisjointFromSiblingPrefixes(t *testing.T) {
	ranges := BuildRangesPrefix("/cadence", "test-ns")
	siblings := []string{
		BuildExecutorsPrefix("/cadence", "test-ns"),
		BuildDrainedShardsPrefix("/cadence", "test-ns"),
		BuildDrainedHostsPrefix("/cadence", "test-ns"),
	}

	for _, sibling := range siblings {
		assert.NotEqual(t, ranges, sibling)
		assert.False(t, strings.HasPrefix(ranges, sibling))
		assert.False(t, strings.HasPrefix(sibling, ranges))
	}
}

// Hex encoding must preserve lexicographic ordering of the bounds. If it did
// not, etcd prefix scans would return ranges out of order, breaking the range
// cache and cover validation.
func TestRangeKeyHexPreservesOrdering(t *testing.T) {
	lower := []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	upper := []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	lowerKey := BuildRangeKey("/cadence", "test-ns", lower)
	upperKey := BuildRangeKey("/cadence", "test-ns", upper)

	assert.True(t, lowerKey < upperKey, "lower bound key must sort before upper bound key")
}

func TestParseRangeKey(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantBound []byte
		wantErr   string
	}{
		{
			name:      "valid",
			key:       "/cadence/test-ns/ranges/83ee861aac655362",
			wantBound: []byte{0x83, 0xee, 0x86, 0x1a, 0xac, 0x65, 0x53, 0x62},
		},
		{
			name:      "min bound",
			key:       "/cadence/test-ns/ranges/0000000000000000",
			wantBound: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:      "max bound",
			key:       "/cadence/test-ns/ranges/ffffffffffffffff",
			wantBound: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:    "wrong prefix",
			key:     "/wrong/prefix/ranges/83ee861aac655362",
			wantErr: "does not have expected ranges prefix",
		},
		{
			name:    "different namespace",
			key:     "/cadence/other-ns/ranges/83ee861aac655362",
			wantErr: "does not have expected ranges prefix",
		},
		{
			name:    "missing lower bound",
			key:     "/cadence/test-ns/ranges/",
			wantErr: "missing lower bound",
		},
		{
			name:    "invalid hex",
			key:     "/cadence/test-ns/ranges/zzzz",
			wantErr: "invalid hex lower bound",
		},
		{
			name:    "odd hex length",
			key:     "/cadence/test-ns/ranges/abc",
			wantErr: "invalid hex lower bound",
		},
		{
			name:    "wrong byte length",
			key:     "/cadence/test-ns/ranges/83ee861aac65536200",
			wantErr: "must be 8 bytes",
		},
		{
			name:    "extra path segment",
			key:     "/cadence/test-ns/ranges/83ee861aac655362/extra",
			wantErr: "invalid hex lower bound",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bound, err := ParseRangeKey("/cadence", "test-ns", tc.key)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, bound)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantBound, bound)
		})
	}
}

func TestRangeKeyRoundTrip(t *testing.T) {
	bounds := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x83, 0xee, 0x86, 0x1a, 0xac, 0x65, 0x53, 0x62},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}

	for _, bound := range bounds {
		t.Run(strings.ToUpper(hex.EncodeToString(bound)), func(t *testing.T) {
			key := BuildRangeKey("/cadence", "test-ns", bound)
			got, err := ParseRangeKey("/cadence", "test-ns", key)
			require.NoError(t, err)
			assert.Equal(t, bound, got)
		})
	}
}
