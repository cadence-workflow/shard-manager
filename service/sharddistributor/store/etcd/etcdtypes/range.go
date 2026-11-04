package etcdtypes

import (
	"encoding/hex"
	"fmt"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

// RangeAssignment is the persisted ownership record for a range.
type RangeAssignment struct {
	LowInclusiveHex string `json:"low_inclusive"`
	// HighExclusiveHex is empty when the range extends to the end of the key space.
	HighExclusiveHex string                 `json:"high_exclusive"`
	ExecutorID       string                 `json:"executor_id"`
	Status           types.AssignmentStatus `json:"status"`
}

// ToRangeAssignment converts the stored record into its internal representation.
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

// FromRangeAssignment converts an internal range assignment to its stored record.
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
