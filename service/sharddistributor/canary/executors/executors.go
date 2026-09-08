package executors

import (
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/processor"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/processorephemeral"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

type ExecutorResult struct {
	fx.Out
	Executor executorclient.Executor[*processor.ShardProcessor] `group:"executor-fixed-proc"`
	Debugger executorclient.Debugger                            `group:"shard-distributor-executors"`
}

type ExecutorEphemeralResult struct {
	fx.Out
	Executor executorclient.Executor[*processorephemeral.ShardProcessor] `group:"executor-ephemeral-proc"`
	Debugger executorclient.Debugger                                     `group:"shard-distributor-executors"`
}

type ExecutorsResult struct {
	fx.Out
	Executors []executorclient.Executor[*processor.ShardProcessor] `group:"executor-fixed-proc,flatten"`
	Debuggers []executorclient.Debugger                            `group:"shard-distributor-executors,flatten"`
}

type ExecutorsEphemeralResult struct {
	fx.Out
	Executors []executorclient.Executor[*processorephemeral.ShardProcessor] `group:"executor-ephemeral-proc,flatten"`
	Debuggers []executorclient.Debugger                                     `group:"shard-distributor-executors,flatten"`
}

func NewExecutorsWithFixedNamespace(params executorclient.Params[*processor.ShardProcessor], namespace string, numExecutors int) (ExecutorsResult, error) {
	var result ExecutorsResult

	if numExecutors <= 0 {
		numExecutors = 1
	}

	for i := 0; i < numExecutors; i++ {
		executor, err := executorclient.NewExecutorWithNamespace(params, namespace)
		if err != nil {
			return ExecutorsResult{}, err
		}
		result.Executors = append(result.Executors, executor)
		result.Debuggers = append(result.Debuggers, executor)
	}

	return result, nil
}

func NewExecutorWithFixedNamespace(params executorclient.Params[*processor.ShardProcessor], namespace string) (ExecutorResult, error) {
	executor, err := executorclient.NewExecutorWithNamespace(params, namespace)
	if err != nil {
		return ExecutorResult{}, err
	}
	return ExecutorResult{Executor: executor, Debugger: executor}, nil
}

func NewExecutorsWithEphemeralNamespace(params executorclient.Params[*processorephemeral.ShardProcessor], namespace string, numExecutors int) (ExecutorsEphemeralResult, error) {
	var result ExecutorsEphemeralResult

	if numExecutors <= 0 {
		numExecutors = 1
	}

	for i := 0; i < numExecutors; i++ {
		executor, err := executorclient.NewExecutorWithNamespace(params, namespace)
		if err != nil {
			return ExecutorsEphemeralResult{}, err
		}
		result.Executors = append(result.Executors, executor)
		result.Debuggers = append(result.Debuggers, executor)
	}

	return result, nil
}

func NewExecutorWithEphemeralNamespace(params executorclient.Params[*processorephemeral.ShardProcessor], namespace string) (ExecutorEphemeralResult, error) {
	executor, err := executorclient.NewExecutorWithNamespace(params, namespace)
	if err != nil {
		return ExecutorEphemeralResult{}, err
	}
	return ExecutorEphemeralResult{Executor: executor, Debugger: executor}, nil
}

type ExecutorsParams struct {
	fx.In
	Lc                 fx.Lifecycle
	ExecutorsFixed     []executorclient.Executor[*processor.ShardProcessor]          `group:"executor-fixed-proc"`
	Executorsephemeral []executorclient.Executor[*processorephemeral.ShardProcessor] `group:"executor-ephemeral-proc"`
}

func NewExecutorsModule(params ExecutorsParams) {
	for _, e := range params.ExecutorsFixed {
		params.Lc.Append(fx.StartStopHook(e.Start, e.Stop))
	}
	for _, e := range params.Executorsephemeral {
		params.Lc.Append(fx.StartStopHook(e.Start, e.Stop))
	}
}

func Module(fixedNamespace, ephemeralNamespace string) fx.Option {
	return fx.Module("Executors",
		// Executor that is used for testing a namespace with fixed shards
		fx.Provide(func(cfg config.Config, params executorclient.Params[*processor.ShardProcessor]) (ExecutorsResult, error) {
			return NewExecutorsWithFixedNamespace(params, fixedNamespace, cfg.Canary.NumFixedExecutors)
		}),

		// Executor that is used for testing a namespaces with ephemeral shards
		fx.Provide(func(cfg config.Config, params executorclient.Params[*processorephemeral.ShardProcessor]) (ExecutorsEphemeralResult, error) {
			return NewExecutorsWithEphemeralNamespace(params, ephemeralNamespace, cfg.Canary.NumEphemeralExecutors)
		}),

		fx.Invoke(NewExecutorsModule),
	)
}
