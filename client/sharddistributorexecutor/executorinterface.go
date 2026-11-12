package sharddistributorexecutor

import (
	"context"

	"go.uber.org/yarpc"

	"github.com/cadence-workflow/shard-manager/common/types"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination executorinterface_mock.go -self_package github.com/cadence-workflow/shard-manager/client/sharddistributorexecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/retry.tmpl -o ../wrappers/retryable/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/metered.tmpl -o ../wrappers/metered/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/errorinjectors.tmpl -o ../wrappers/errorinjectors/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/grpc.tmpl -o ../wrappers/grpc/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor -v package=apiv1 -v path=github.com/cadence-workflow/shard-manager/proto/internal/uber/cadence/sharddistributor/v1 -v prefix=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/timeout.tmpl -o ../wrappers/timeout/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor
//go:generate gowrap gen -g -p . -i Client -t ../templates/hedge.tmpl -o ../wrappers/hedged/sharddistributorexecutor_generated.go -v client=ShardDistributorExecutor -v operation=ShardDistributorExecutor

type Client interface {
	Heartbeat(context.Context, *types.ExecutorHeartbeatRequest, ...yarpc.CallOption) (*types.ExecutorHeartbeatResponse, error)
}
