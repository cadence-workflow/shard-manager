package clusterredirection

import (
	"context"

	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/common/client"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func getRequestedConsistencyLevelFromContext(ctx context.Context) types.QueryConsistencyLevel {
	call := yarpc.CallFromContext(ctx)
	if call == nil {
		return types.QueryConsistencyLevelEventual
	}
	featureFlags := client.GetFeatureFlagsFromHeader(call)
	if featureFlags.AutoforwardingEnabled {
		return types.QueryConsistencyLevelStrong
	}
	return types.QueryConsistencyLevelEventual
}
