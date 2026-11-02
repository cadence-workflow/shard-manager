package etcdtypes

import (
	"encoding/hex"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// RangeAssignment is the persisted ownership record for one range, stored in
// etcd under ranges/<rangeID> with both bounds hex-encoded in the value.
type RangeAssignment struct {
	// LowInclusiveHex is the hex-encoded inclusive lower bound.
	LowInclusiveHex string `json:"low_inclusive"`
	// HighExclusiveHex is the hex-encoded exclusive upper bound, or the empty
	// string when the range extends to the end of the key space.
	HighExclusiveHex string                 `json:"high_exclusive"`
	ExecutorID       string                 `json:"executor_id"`
	Status           types.AssignmentStatus `json:"status"`
}

// ToRangeAssignment converts the stored record plus its range ID into the
// internal representation.
func (r *RangeAssignment) ToRangeAssignment(rangeID string) (*store.RangeAssignment, error) {
	if r == nil {
		return nil, nil
	}

	lowInclusive, err := hex.DecodeString(r.LowInclusiveHex)
	if err != nil {
		return nil, fmt.Errorf("decode low bound %q: %w", r.LowInclusiveHex, err)
	}

	var rng types.Range
	if r.HighExclusiveHex == "" {
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
		ID:         rangeID,
		Range:      rng,
		ExecutorID: r.ExecutorID,
		Status:     r.Status,
	}, nil
}

// FromRangeAssignment converts an internal range assignment to its stored
// record. The caller must ensure src.Range.IsValid(); otherwise the
// resulting record cannot be decoded by ToRangeAssignment.
func FromRangeAssignment(src *store.RangeAssignment) *RangeAssignment {
	if src == nil {
		return nil
	}

	var highExclusiveHex string
	if !src.Range.IsFinalRange() {
		highExclusiveHex = hex.EncodeToString(src.Range.HighExclusive)
	}

	return &RangeAssignment{
		LowInclusiveHex:  hex.EncodeToString(src.Range.LowInclusive),
		HighExclusiveHex: highExclusiveHex,
		ExecutorID:       src.ExecutorID,
		Status:           src.Status,
	}
}
