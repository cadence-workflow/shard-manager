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
	"fmt"
	"time"
)

type (
	// DynamicInt defines the properties for a dynamic config with int value type
	DynamicInt struct {
		KeyName      string
		Description  string
		DefaultValue int
		Filters      []Filter
	}

	DynamicBool struct {
		KeyName      string
		Description  string
		DefaultValue bool
		Filters      []Filter
	}

	DynamicFloat struct {
		KeyName      string
		Description  string
		DefaultValue float64
		Filters      []Filter
	}

	DynamicString struct {
		KeyName      string
		Description  string
		DefaultValue string
		Filters      []Filter
	}

	DynamicDuration struct {
		KeyName      string
		Description  string
		DefaultValue time.Duration
		Filters      []Filter
	}

	DynamicMap struct {
		KeyName      string
		Description  string
		DefaultValue map[string]interface{}
		Filters      []Filter
	}

	DynamicList struct {
		KeyName      string
		Description  string
		DefaultValue []interface{}
		Filters      []Filter
	}

	IntKey      int
	BoolKey     int
	FloatKey    int
	StringKey   int
	DurationKey int
	MapKey      int
	ListKey     int

	Key interface {
		String() string
		Description() string
		DefaultValue() interface{}
		// Filters is used to identify what filters a DynamicConfig key may have.
		// For example, CLI tool uses this to figure out all domain specific configurations for migration validation.
		Filters() []Filter
	}
)

// ListAllProductionKeys returns all key used in production
func ListAllProductionKeys() []Key {
	result := make([]Key, 0, len(IntKeys)+len(BoolKeys)+len(FloatKeys)+len(StringKeys)+len(DurationKeys)+len(MapKeys))
	for i := TestGetIntPropertyFilteredByShardIDKey + 1; i < LastIntKey; i++ {
		result = append(result, i)
	}
	for i := TestGetBoolPropertyFilteredByShardIDKey + 1; i < LastBoolKey; i++ {
		result = append(result, i)
	}
	for i := TestGetFloat64PropertyFilteredByShardIDKey + 1; i < LastFloatKey; i++ {
		result = append(result, i)
	}
	for i := TestGetStringPropertyKey + 1; i < LastStringKey; i++ {
		result = append(result, i)
	}
	for i := TestGetDurationPropertyFilteredByShardID + 1; i < LastDurationKey; i++ {
		result = append(result, i)
	}
	for i := TestGetMapPropertyKey + 1; i < LastMapKey; i++ {
		result = append(result, i)
	}
	for i := TestGetListPropertyKey + 1; i < LastListKey; i++ {
		result = append(result, i)
	}
	return result
}

func GetKeyFromKeyName(keyName string) (Key, error) {
	keyVal, ok := _keyNames[keyName]
	if !ok {
		return nil, fmt.Errorf("invalid dynamic config key name: %s", keyName)
	}
	return keyVal, nil
}

// GetAllKeys returns a copy of all configuration keys with all details
func GetAllKeys() map[string]Key {
	result := make(map[string]Key, len(_keyNames))
	for k, v := range _keyNames {
		result[k] = v
	}
	return result
}

func ValidateKeyValuePair(key Key, value interface{}) error {
	err := fmt.Errorf("key value pair mismatch, key type: %T, value type: %T", key, value)
	switch key.(type) {
	case IntKey:
		if _, ok := value.(int); !ok {
			return err
		}
	case BoolKey:
		if _, ok := value.(bool); !ok {
			return err
		}
	case FloatKey:
		if _, ok := value.(float64); !ok {
			return err
		}
	case StringKey:
		if _, ok := value.(string); !ok {
			return err
		}
	case DurationKey:
		if _, ok := value.(time.Duration); !ok {
			return err
		}
	case MapKey:
		if _, ok := value.(map[string]interface{}); !ok {
			return err
		}
	case ListKey:
		if _, ok := value.([]interface{}); !ok {
			return err
		}
	default:
		return fmt.Errorf("unknown key type: %T", key)
	}
	return nil
}

func (k IntKey) String() string {
	return IntKeys[k].KeyName
}

func (k IntKey) Description() string {
	return IntKeys[k].Description
}

func (k IntKey) DefaultValue() interface{} {
	return IntKeys[k].DefaultValue
}

func (k IntKey) DefaultInt() int {
	return IntKeys[k].DefaultValue
}

func (k IntKey) Filters() []Filter {
	return IntKeys[k].Filters
}

func (k BoolKey) String() string {
	return BoolKeys[k].KeyName
}

func (k BoolKey) Description() string {
	return BoolKeys[k].Description
}

func (k BoolKey) DefaultValue() interface{} {
	return BoolKeys[k].DefaultValue
}

func (k BoolKey) DefaultBool() bool {
	return BoolKeys[k].DefaultValue
}

func (k BoolKey) Filters() []Filter {
	return BoolKeys[k].Filters
}

func (k FloatKey) String() string {
	return FloatKeys[k].KeyName
}

func (k FloatKey) Description() string {
	return FloatKeys[k].Description
}

func (k FloatKey) DefaultValue() interface{} {
	return FloatKeys[k].DefaultValue
}

func (k FloatKey) DefaultFloat() float64 {
	return FloatKeys[k].DefaultValue
}

func (k FloatKey) Filters() []Filter {
	return FloatKeys[k].Filters
}

func (k StringKey) String() string {
	return StringKeys[k].KeyName
}

func (k StringKey) Description() string {
	return StringKeys[k].Description
}

func (k StringKey) DefaultValue() interface{} {
	return StringKeys[k].DefaultValue
}

func (k StringKey) DefaultString() string {
	return StringKeys[k].DefaultValue
}

func (k StringKey) Filters() []Filter {
	return StringKeys[k].Filters
}

func (k DurationKey) String() string {
	return DurationKeys[k].KeyName
}

func (k DurationKey) Description() string {
	return DurationKeys[k].Description
}

func (k DurationKey) DefaultValue() interface{} {
	return DurationKeys[k].DefaultValue
}

func (k DurationKey) DefaultDuration() time.Duration {
	return DurationKeys[k].DefaultValue
}

func (k DurationKey) Filters() []Filter {
	return DurationKeys[k].Filters
}

func (k MapKey) String() string {
	return MapKeys[k].KeyName
}

func (k MapKey) Description() string {
	return MapKeys[k].Description
}

func (k MapKey) DefaultValue() interface{} {
	return MapKeys[k].DefaultValue
}

func (k MapKey) DefaultMap() map[string]interface{} {
	return MapKeys[k].DefaultValue
}

func (k MapKey) Filters() []Filter {
	return MapKeys[k].Filters
}

func (k ListKey) String() string {
	return ListKeys[k].KeyName
}

func (k ListKey) Description() string {
	return ListKeys[k].Description
}

func (k ListKey) DefaultValue() interface{} {
	return ListKeys[k].DefaultValue
}

func (k ListKey) DefaultList() []interface{} {
	return ListKeys[k].DefaultValue
}

func (k ListKey) Filters() []Filter {
	return ListKeys[k].Filters
}

const (
	UnknownIntKey IntKey = iota

	// key for tests
	TestGetIntPropertyKey
	TestGetIntPropertyFilteredByDomainKey
	TestGetIntPropertyFilteredByWorkflowTypeKey
	TestGetIntPropertyFilteredByTaskListInfoKey
	TestGetIntPropertyFilteredByShardIDKey

	// ShardDistributorMaxEtcdTxnOps is the maximum number of operations per etcd transaction.
	// etcd enforces a server-side limit (--max-txn-ops, default 128).
	// This value must not exceed the etcd cluster's configured limit.
	// KeyName: shardDistributor.maxEtcdTxnOps
	// Value type: Int
	// Default value: 128
	// Allowed filters: N/A
	ShardDistributorMaxEtcdTxnOps

	// LastIntKey must be the last one in this const group
	LastIntKey
)

const (
	UnknownBoolKey BoolKey = iota

	// key for tests
	TestGetBoolPropertyKey
	TestGetBoolPropertyFilteredByDomainIDKey
	TestGetBoolPropertyFilteredByTaskListInfoKey
	TestGetBoolPropertyFilteredByDomainKey
	TestGetBoolPropertyFilteredByDomainIDAndWorkflowIDKey
	TestGetBoolPropertyFilteredByShardIDKey

	// LastBoolKey must be the last one in this const group
	LastBoolKey
)

const (
	UnknownFloatKey FloatKey = iota

	// key for tests
	TestGetFloat64PropertyKey
	TestGetFloat64PropertyFilteredByShardIDKey

	// ShardDistributorErrorInjectionRate is rate for injecting random error in shard distributor client
	// KeyName: sharddistributor.errorInjectionRate
	// Value type: Float64
	// Default value: 0
	// Allowed filters: N/A
	ShardDistributorErrorInjectionRate

	// ShardDistributorExecutorErrorInjectionRate is rate for injecting random error in shard distributor executor client
	// KeyName: sharddistributorexecutor.errorInjectionRate
	// Value type: Float64
	// Default value: 0
	// Allowed filters: N/A
	ShardDistributorExecutorErrorInjectionRate

	// ShardDistributorLoadBalancingNaiveMaxDeviation is max deviation between the coldest and hottest executors
	// in naive load balancing mode
	//
	// KeyName: shardDistributor.loadBalancingNaive.maxDeviation
	// Value type: Float64
	// Default value: 2.0
	// Allowed filters: namespace
	ShardDistributorLoadBalancingNaiveMaxDeviation

	// ShardDistributorLoadBalancingGreedyMoveBudgetProportion is the fraction of total shards
	// that may be moved per greedy load-balance pass.
	//
	// KeyName: shardDistributor.loadBalancingGreedy.moveBudgetProportion
	// Value type: Float64
	// Default value: 0.01
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedyMoveBudgetProportion

	// ShardDistributorLoadBalancingGreedyHysteresisUpperBand is the multiplier above mean load
	// that qualifies an executor as a greedy rebalance source.
	//
	// KeyName: shardDistributor.loadBalancingGreedy.hysteresisUpperBand
	// Value type: Float64
	// Default value: 1.15
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedyHysteresisUpperBand

	// ShardDistributorLoadBalancingGreedyHysteresisLowerBand is the multiplier below mean load
	// that qualifies an executor as a greedy rebalance destination.
	//
	// KeyName: shardDistributor.loadBalancingGreedy.hysteresisLowerBand
	// Value type: Float64
	// Default value: 0.90
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedyHysteresisLowerBand

	// ShardDistributorLoadBalancingGreedySevereImbalanceRatio allows relaxing destination
	// selection when maxLoad/meanLoad reaches this value.
	//
	// KeyName: shardDistributor.loadBalancingGreedy.severeImbalanceRatio
	// Value type: Float64
	// Default value: 1.3
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedySevereImbalanceRatio

	// LastFloatKey must be the last one in this const group
	LastFloatKey
)

const (
	UnknownStringKey StringKey = iota

	// key for tests
	TestGetStringPropertyKey

	// ShardDistributorLoadBalancingMode is the load balancing mode for the shard distributor
	// Depending on the mode, the shard distributor will use different ways to distribute the shards
	//
	// * "naive" 	- mode assigns shards to the least loaded hosts without considering the existing shard distribution
	// * "greedy" 	- mode balances the load across all hosts while minimizing shard movements and uses shard statistics to make better decisions
	//
	// KeyName: shardDistributor.loadBalancingMode
	// Value type: String
	// Default value: "naive"
	// Allowed filters: namespace
	ShardDistributorLoadBalancingMode

	// LastStringKey must be the last one in this const group
	LastStringKey
)

const (
	UnknownDurationKey DurationKey = iota

	// key for tests
	TestGetDurationPropertyKey
	TestGetDurationPropertyFilteredByDomainKey
	TestGetDurationPropertyFilteredByTaskListInfoKey
	TestGetDurationPropertyFilteredByWorkflowTypeKey
	TestGetDurationPropertyFilteredByDomainIDKey
	TestGetDurationPropertyFilteredByShardID

	// ShardDistributorLoadBalancingGreedyPerShardCooldown is the minimum time between
	// moving the same shard in greedy load balancing mode.
	// KeyName: shardDistributor.loadBalancingGreedy.perShardCooldown
	// Value type: Duration
	// Default value: 1 minute
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedyPerShardCooldown

	// ShardDistributorLoadBalancingGreedyLoadSmoothingTimeConstant is the time constant
	// for exponential smoothing of shard load in greedy load balancing mode.
	// KeyName: shardDistributor.loadBalancingGreedy.loadSmoothingTimeConstant
	// Value type: Duration
	// Default value: 1 minute
	// Allowed filters: namespace
	ShardDistributorLoadBalancingGreedyLoadSmoothingTimeConstant

	// ShardDistributorEphemeralAssignmentCoalescingWindow is how long the shard
	// distributor collects initial-assignment requests before processing a batch.
	// KeyName: shardDistributor.ephemeralAssignment.coalescingWindow
	// Value type: Duration
	// Default value: 10 milliseconds
	// Allowed filters: namespace
	ShardDistributorEphemeralAssignmentCoalescingWindow

	// LastDurationKey must be the last one in this const group
	LastDurationKey
)

const (
	UnknownMapKey MapKey = iota
	TestGetMapPropertyKey
	LastMapKey
)

const (
	UnknownListKey ListKey = iota
	TestGetListPropertyKey
	LastListKey
)

var IntKeys = map[IntKey]DynamicInt{
	TestGetIntPropertyKey: {
		KeyName:      "testGetIntPropertyKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetIntPropertyFilteredByDomainKey: {
		KeyName:      "testGetIntPropertyFilteredByDomainKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetIntPropertyFilteredByTaskListInfoKey: {
		KeyName:      "testGetIntPropertyFilteredByTaskListInfoKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetIntPropertyFilteredByWorkflowTypeKey: {
		KeyName:      "testGetIntPropertyFilteredByWorkflowTypeKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetIntPropertyFilteredByShardIDKey: {
		KeyName:      "testGetIntPropertyFilteredByShardIDKey",
		Description:  "",
		DefaultValue: 0,
		Filters:      nil,
	},
	ShardDistributorMaxEtcdTxnOps: {
		KeyName:      "shardDistributor.maxEtcdTxnOps",
		Description:  "ShardDistributorMaxEtcdTxnOps is the maximum number of operations per etcd transaction, must not exceed the etcd cluster's configured --max-txn-ops limit",
		DefaultValue: 128,
	},
}

var BoolKeys = map[BoolKey]DynamicBool{
	TestGetBoolPropertyKey: {
		KeyName:      "testGetBoolPropertyKey",
		Description:  "",
		DefaultValue: false,
	},
	TestGetBoolPropertyFilteredByDomainIDKey: {
		KeyName:      "testGetBoolPropertyFilteredByDomainIDKey",
		Description:  "",
		DefaultValue: false,
	},
	TestGetBoolPropertyFilteredByTaskListInfoKey: {
		KeyName:      "testGetBoolPropertyFilteredByTaskListInfoKey",
		Description:  "",
		DefaultValue: false,
	},
	TestGetBoolPropertyFilteredByDomainKey: {
		KeyName:      "testGetBoolPropertyFilteredByDomainKey",
		Description:  "",
		DefaultValue: false,
		Filters:      nil,
	},
	TestGetBoolPropertyFilteredByDomainIDAndWorkflowIDKey: {
		KeyName:      "testGetBoolPropertyFilteredByDomainIDandWorkflowIDKey",
		Description:  "",
		DefaultValue: false,
		Filters:      nil,
	},
	TestGetBoolPropertyFilteredByShardIDKey: {
		KeyName:      "testGetBoolPropertyFilteredByShardIDKey",
		Description:  "",
		DefaultValue: false,
		Filters:      []Filter{ShardID},
	},
}

var FloatKeys = map[FloatKey]DynamicFloat{
	TestGetFloat64PropertyKey: {
		KeyName:      "testGetFloat64PropertyKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetFloat64PropertyFilteredByShardIDKey: {
		KeyName:      "testGetFloat64PropertyFilteredByShardIDKey",
		Description:  "",
		DefaultValue: 0,
		Filters:      nil,
	},
	ShardDistributorErrorInjectionRate: {
		KeyName:      "sharddistributor.errorInjectionRate",
		Description:  "ShardDistributorInjectionRate is rate for injecting random error in shard distributor client",
		DefaultValue: 0,
	},
	ShardDistributorExecutorErrorInjectionRate: {
		KeyName:      "sharddistributorexecutor.errorInjectionRate",
		Description:  "ShardDistributorExecutorInjectionRate is rate for injecting random error in shard distributor executor client",
		DefaultValue: 0,
	},
	ShardDistributorLoadBalancingNaiveMaxDeviation: {
		KeyName:      "shardDistributor.loadBalancingNaive.maxDeviation",
		Description:  "ShardDistributorLoadBalancingNaiveMaxDeviation is max deviation between the coldest and hottest executors in naive load balancing mode",
		DefaultValue: 2.0,
		Filters:      []Filter{Namespace},
	},
	ShardDistributorLoadBalancingGreedyMoveBudgetProportion: {
		KeyName:      "shardDistributor.loadBalancingGreedy.moveBudgetProportion",
		Description:  "ShardDistributorLoadBalancingGreedyMoveBudgetProportion is the fraction of total shards that may be moved per greedy load-balance pass",
		DefaultValue: 0.01,
		Filters:      []Filter{Namespace},
	},
	ShardDistributorLoadBalancingGreedyHysteresisUpperBand: {
		KeyName:      "shardDistributor.loadBalancingGreedy.hysteresisUpperBand",
		Description:  "ShardDistributorLoadBalancingGreedyHysteresisUpperBand is the multiplier above mean load that qualifies an executor as a greedy rebalance source",
		DefaultValue: 1.15,
		Filters:      []Filter{Namespace},
	},
	ShardDistributorLoadBalancingGreedyHysteresisLowerBand: {
		KeyName:      "shardDistributor.loadBalancingGreedy.hysteresisLowerBand",
		Description:  "ShardDistributorLoadBalancingGreedyHysteresisLowerBand is the multiplier below mean load that qualifies an executor as a greedy rebalance destination",
		DefaultValue: 0.90,
		Filters:      []Filter{Namespace},
	},
	ShardDistributorLoadBalancingGreedySevereImbalanceRatio: {
		KeyName:      "shardDistributor.loadBalancingGreedy.severeImbalanceRatio",
		Description:  "ShardDistributorLoadBalancingGreedySevereImbalanceRatio allows relaxing destination selection when maxLoad/meanLoad reaches this value",
		DefaultValue: 1.3,
		Filters:      []Filter{Namespace},
	},
}

var StringKeys = map[StringKey]DynamicString{
	TestGetStringPropertyKey: {
		KeyName:      "testGetStringPropertyKey",
		Description:  "",
		DefaultValue: "",
	},
	ShardDistributorLoadBalancingMode: {
		KeyName:      "shardDistributor.loadBalancingMode",
		Description:  "ShardDistributorLoadBalancingMode is the load balancing mode for the shard distributor. Depending on the mode, the shard distributor will use different ways to distribute the shards",
		DefaultValue: "naive",
	},
}

var DurationKeys = map[DurationKey]DynamicDuration{
	TestGetDurationPropertyKey: {
		KeyName:      "testGetDurationPropertyKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetDurationPropertyFilteredByDomainKey: {
		KeyName:      "testGetDurationPropertyFilteredByDomainKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetDurationPropertyFilteredByTaskListInfoKey: {
		KeyName:      "testGetDurationPropertyFilteredByTaskListInfoKey",
		Description:  "",
		DefaultValue: 0,
	},
	TestGetDurationPropertyFilteredByWorkflowTypeKey: {
		KeyName:      "testGetDurationPropertyFilteredByWorkflowTypeKey",
		Description:  "",
		DefaultValue: 0,
		Filters:      nil,
	},
	TestGetDurationPropertyFilteredByDomainIDKey: {
		KeyName:      "testGetDurationPropertyFilteredByDomainIDKey",
		Description:  "",
		DefaultValue: 0,
		Filters:      nil,
	},
	TestGetDurationPropertyFilteredByShardID: {
		KeyName:      "testGetDurationPropertyFilteredByShardID",
		Description:  "",
		DefaultValue: 0,
		Filters:      nil,
	},
	ShardDistributorLoadBalancingGreedyPerShardCooldown: {
		KeyName:      "shardDistributor.loadBalancingGreedy.perShardCooldown",
		Filters:      []Filter{Namespace},
		Description:  "ShardDistributorLoadBalancingGreedyPerShardCooldown is the minimum time between moving the same shard in greedy load balancing mode",
		DefaultValue: time.Minute,
	},
	ShardDistributorLoadBalancingGreedyLoadSmoothingTimeConstant: {
		KeyName:      "shardDistributor.loadBalancingGreedy.loadSmoothingTimeConstant",
		Filters:      []Filter{Namespace},
		Description:  "ShardDistributorLoadBalancingGreedyLoadSmoothingTimeConstant is the time constant for exponential smoothing of shard load in greedy load balancing mode",
		DefaultValue: time.Minute,
	},
	ShardDistributorEphemeralAssignmentCoalescingWindow: {
		KeyName:      "shardDistributor.ephemeralAssignment.coalescingWindow",
		Filters:      []Filter{Namespace},
		Description:  "ShardDistributorEphemeralAssignmentCoalescingWindow is how long the shard distributor collects initial-assignment requests before processing a batch",
		DefaultValue: 10 * time.Millisecond,
	},
}

var MapKeys = map[MapKey]DynamicMap{
	TestGetMapPropertyKey: {
		KeyName:      "testGetMapPropertyKey",
		Description:  "",
		DefaultValue: nil,
	},
}

var ListKeys = map[ListKey]DynamicList{
	TestGetListPropertyKey: {
		KeyName:      "testGetListPropertyKey",
		Description:  "",
		DefaultValue: nil,
	},
}

var _keyNames map[string]Key

func init() {
	panicIfKeyInvalid := func(name string, key Key) {
		if name == "" {
			panic(fmt.Sprintf("empty keyName: %T, %v", key, key))
		}
		if _, ok := _keyNames[name]; ok {
			panic(fmt.Sprintf("duplicate keyName: %v", name))
		}
	}
	_keyNames = make(map[string]Key)
	for k, v := range IntKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range BoolKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range FloatKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range StringKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range DurationKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range MapKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
	for k, v := range ListKeys {
		panicIfKeyInvalid(v.KeyName, k)
		_keyNames[v.KeyName] = k
	}
}
