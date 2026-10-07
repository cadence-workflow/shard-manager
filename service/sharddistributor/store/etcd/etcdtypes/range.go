package etcdtypes

import (
	"encoding/hex"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// RangeAssignment is the persisted ownership record for one range. It is stored
// in etcd with the lower bound as the key and the rest of the record as the
// value. The lower bound is not stored in the record because it is already part
// of the key.
type RangeAssignment struct {
	// HighExclusiveHex is the hex-encoded exclusive upper bound, or the empty
	// string when the range extends to the end of the key space.
	HighExclusiveHex string                 `json:"high_exclusive"`
	ExecutorID       string                 `json:"executor_id"`
	Status           types.AssignmentStatus `json:"status"`
}

// ToRangeAssignment converts the stored record plus its lower bound into the
// internal representation.
func (r *RangeAssignment) ToRangeAssignment(lowInclusive []byte) (*store.RangeAssignment, error) {
	if r == nil {
		return nil, nil
	}

	var rng *types.Range
	if r.HighExclusiveHex == "" {
		var err error
		rng, err = types.NewFinalRange(lowInclusive)
		if err != nil {
			return nil, fmt.Errorf("build final range: %w", err)
		}
	} else {
		highExclusive, err := hex.DecodeString(r.HighExclusiveHex)
		if err != nil {
			return nil, fmt.Errorf("decode high bound %q: %w", r.HighExclusiveHex, err)
		}
		rng, err = types.NewRange(lowInclusive, highExclusive)
		if err != nil {
			return nil, fmt.Errorf("build range: %w", err)
		}
	}

	return &store.RangeAssignment{
		Range:      rng,
		ExecutorID: r.ExecutorID,
		Status:     r.Status,
	}, nil
}

// FromRangeAssignment converts an internal range assignment into the stored
// record.
func FromRangeAssignment(src *store.RangeAssignment) *RangeAssignment {
	if src == nil {
		return nil
	}

	var highExclusiveHex string
	if src.Range != nil && !src.Range.IsFinalRange() {
		highExclusiveHex = hex.EncodeToString(src.Range.HighExclusive)
	}

	return &RangeAssignment{
		HighExclusiveHex: highExclusiveHex,
		ExecutorID:       src.ExecutorID,
		Status:           src.Status,
	}
}
