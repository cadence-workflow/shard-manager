package types

import (
	"sort"
	"unsafe"
)

type GetDynamicConfigRequest struct {
	ConfigName string                 `json:"configName,omitempty"`
	Filters    []*DynamicConfigFilter `json:"filters,omitempty"`
}

type IsolationGroupState int

const (
	IsolationGroupStateInvalid IsolationGroupState = iota
	IsolationGroupStateHealthy
	IsolationGroupStateDrained
)

type IsolationGroupPartition struct {
	Name  string
	State IsolationGroupState
}

// ByteSize returns an approximate size of the object in bytes
func (i IsolationGroupPartition) ByteSize() uint64 {
	var size uint64
	size += uint64(unsafe.Sizeof(i))
	size += uint64(len(i.Name))
	return size
}

// IsolationGroupConfiguration is an internal representation of a set of
// isolation-groups as a mapping and may refer to either globally or per-domain (or both) configurations.
// and their statuses. It's redundantly indexed by IsolationGroup name to simplify lookups.
//
// For example: This might be a global configuration persisted
// in the config store and look like this:
//
//	IsolationGroupConfiguration{
//	  "isolationGroup1234": {Name: "isolationGroup1234", Status: IsolationGroupStatusDrained},
//	}
//
// Indicating that task processing isn't to occur within this isolationGroup anymore, but all others are ok.
type IsolationGroupConfiguration map[string]IsolationGroupPartition

// ToPartitionList Renders the isolation group to the less complicated and confusing simple list of isolation groups
func (i IsolationGroupConfiguration) ToPartitionList() []IsolationGroupPartition {
	out := []IsolationGroupPartition{}
	for _, v := range i {
		out = append(out, v)
	}
	// ensure determinitism in list ordering for convenience
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (i IsolationGroupConfiguration) DeepCopy() IsolationGroupConfiguration {
	if i == nil {
		return nil
	}

	out := IsolationGroupConfiguration{}
	for k, v := range i {
		out[k] = v
	}
	return out
}

// ByteSize returns an approximate size of the object in bytes
func (i *IsolationGroupConfiguration) ByteSize() uint64 {
	if i == nil {
		return 0
	}

	size := uint64(unsafe.Sizeof(*i))
	for k, v := range *i {
		size += uint64(len(k)) + uint64(len(v.Name))
	}
	return size
}
