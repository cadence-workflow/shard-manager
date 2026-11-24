package executors

import (
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/faultlog"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/processor"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

type ExecutorResult struct {
	fx.Out
	Executor     executorclient.Executor[*processor.ShardProcessor] `group:"executor-fixed-proc"`
	ExecutorInfo executorclient.ExecutorInfo                        `group:"shard-distributor-executors"`
}

type ExecutorsResult struct {
	fx.Out
	Executors     []executorclient.Executor[*processor.ShardProcessor] `group:"executor-fixed-proc,flatten"`
	ExecutorInfos []executorclient.ExecutorInfo                        `group:"shard-distributor-executors,flatten"`
}

func NewExecutorsWithFixedNamespace(params executorclient.Params[*processor.ShardProcessor], cfg config.NamespaceConfig) (ExecutorsResult, error) {
	var result ExecutorsResult

	numExecutors := cfg.NumExecutors
	if numExecutors <= 0 {
		numExecutors = 1
	}

	for i := 0; i < numExecutors; i++ {
		executor, err := executorclient.NewExecutorWithNamespace(params, cfg.Name)
		if err != nil {
			return ExecutorsResult{}, err
		}
		result.Executors = append(result.Executors, executor)
		result.ExecutorInfos = append(result.ExecutorInfos, executor)
	}

	return result, nil
}

func NewExecutorWithFixedNamespace(params executorclient.Params[*processor.ShardProcessor], namespace string) (ExecutorResult, error) {
	executor, err := executorclient.NewExecutorWithNamespace(params, namespace)
	if err != nil {
		return ExecutorResult{}, err
	}
	return ExecutorResult{Executor: executor, ExecutorInfo: executor}, nil
}

type ExecutorsParams struct {
	fx.In
	Lc             fx.Lifecycle
	ExecutorsFixed []executorclient.Executor[*processor.ShardProcessor] `group:"executor-fixed-proc"`
}

func NewExecutorsModule(params ExecutorsParams) {
	for _, e := range params.ExecutorsFixed {
		params.Lc.Append(fx.StartStopHook(e.Start, e.Stop))
	}
}

func Module(fixed config.NamespaceConfig) fx.Option {
	return fx.Module("Executors",
		// The canary stalls Start/Stop on purpose, so the framework's lifecycle
		// timeouts are expected output here rather than actionable failures.
		fx.Decorate(faultlog.WrapLogger),

		// Executor that is used for testing a namespace with fixed shards
		fx.Provide(func(params executorclient.Params[*processor.ShardProcessor]) (ExecutorsResult, error) {
			return NewExecutorsWithFixedNamespace(params, fixed)
		}),

		fx.Invoke(NewExecutorsModule),
	)
}
