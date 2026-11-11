package proto

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/common/types/testdata"
)

func TestHealthResponse(t *testing.T) {
	for _, item := range []*types.HealthStatus{nil, {}, &testdata.HealthStatus} {
		assert.Equal(t, item, ToHealthResponse(FromHealthResponse(item)))
	}
}
