// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package dynamicproperties

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAllProductionKeys(t *testing.T) {
	keys := ListAllProductionKeys()
	require.Len(t, keys, 12)
	for _, key := range keys {
		assert.Contains(t, strings.ToLower(key.String()), "sharddistributor")
	}
}

func TestGetKeyFromKeyName(t *testing.T) {
	key, err := GetKeyFromKeyName("shardDistributor.maxEtcdTxnOps")
	require.NoError(t, err)
	assert.Equal(t, ShardDistributorMaxEtcdTxnOps, key)

	key, err = GetKeyFromKeyName("not-a-key")
	assert.Error(t, err)
	assert.Nil(t, key)
}

func TestGetAllKeys(t *testing.T) {
	keys := GetAllKeys()
	assert.Len(t, keys, len(IntKeys)+len(BoolKeys)+len(FloatKeys)+len(StringKeys)+len(DurationKeys)+len(MapKeys)+len(ListKeys))
	assert.Equal(t, TestGetIntPropertyKey, keys["testGetIntPropertyKey"])
	assert.Equal(t, ShardDistributorLoadBalancingMode, keys["shardDistributor.loadBalancingMode"])
}

type NewKey int

func (k NewKey) String() string            { return "NewKey" }
func (k NewKey) Description() string       { return "NewKey is a new key" }
func (k NewKey) DefaultValue() interface{} { return 0 }
func (k NewKey) Filters() []Filter         { return nil }

func TestValidateKeyValuePair(t *testing.T) {
	tests := []struct {
		name  string
		key   Key
		value interface{}
	}{
		{name: "unknown key", key: NewKey(0), value: 0},
		{name: "int", key: TestGetIntPropertyKey, value: "0"},
		{name: "bool", key: TestGetBoolPropertyKey, value: 0},
		{name: "float", key: TestGetFloat64PropertyKey, value: 0},
		{name: "string", key: TestGetStringPropertyKey, value: 0},
		{name: "duration", key: TestGetDurationPropertyKey, value: 0},
		{name: "map", key: TestGetMapPropertyKey, value: 0},
		{name: "list", key: TestGetListPropertyKey, value: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, ValidateKeyValuePair(tt.key, tt.value))
		})
	}
}

func TestShardDistributorKeys(t *testing.T) {
	tests := []struct {
		name         string
		key          Key
		keyName      string
		defaultValue interface{}
		filters      []Filter
	}{
		{
			name:         "max etcd transaction operations",
			key:          ShardDistributorMaxEtcdTxnOps,
			keyName:      "shardDistributor.maxEtcdTxnOps",
			defaultValue: 128,
		},
		{
			name:         "load balancing mode",
			key:          ShardDistributorLoadBalancingMode,
			keyName:      "shardDistributor.loadBalancingMode",
			defaultValue: "naive",
		},
		{
			name:         "naive max deviation",
			key:          ShardDistributorLoadBalancingNaiveMaxDeviation,
			keyName:      "shardDistributor.loadBalancingNaive.maxDeviation",
			defaultValue: 2.0,
			filters:      []Filter{Namespace},
		},
		{
			name:         "greedy per-shard cooldown",
			key:          ShardDistributorLoadBalancingGreedyPerShardCooldown,
			keyName:      "shardDistributor.loadBalancingGreedy.perShardCooldown",
			defaultValue: time.Minute,
			filters:      []Filter{Namespace},
		},
		{
			name:         "ephemeral assignment coalescing window",
			key:          ShardDistributorEphemeralAssignmentCoalescingWindow,
			keyName:      "shardDistributor.ephemeralAssignment.coalescingWindow",
			defaultValue: 10 * time.Millisecond,
			filters:      []Filter{Namespace},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.keyName, tt.key.String())
			assert.Equal(t, tt.defaultValue, tt.key.DefaultValue())
			assert.Equal(t, tt.filters, tt.key.Filters())
		})
	}
}

func TestDynamicConfigFilterTypeIsMapped(t *testing.T) {
	require.Equal(t, int(LastFilterTypeForTest), len(filters))
	for i := UnknownFilter; i < LastFilterTypeForTest; i++ {
		require.NotEmpty(t, filters[i])
	}
}

func TestDynamicConfigFilterTypeIsParseable(t *testing.T) {
	allFilters := map[Filter]int{}
	for idx, filterString := range filters {
		parsed := ParseFilter(filterString)
		prev, ok := allFilters[parsed]
		assert.False(t, ok, "%q is already mapped to the same filter type as %q", filterString, filters[prev])
		allFilters[parsed] = idx

		if idx == 0 {
			assert.Equal(t, UnknownFilter, parsed)
			require.Equal(t, "unknownFilter", filterString)
		} else {
			assert.NotEqual(t, UnknownFilter, parsed)
		}
	}
}

func TestDynamicConfigFilterStringsCorrectly(t *testing.T) {
	for _, filterString := range filters {
		parsed := ParseFilter(filterString)
		assert.Equal(t, filterString, parsed.String())
	}
	badFilter := Filter(len(filters))
	assert.Equal(t, UnknownFilter.String(), badFilter.String())
}
