package tasklist

import (
	"time"

	"github.com/cadence-workflow/shard-manager/common/clock"
)

// ShardProcessorFactory is a generic factory for creating ShardProcessor instances.
type ShardProcessorFactory struct {
	TaskListsRegistry TaskListRegistry
	ReportTTL         time.Duration
	TimeSource        clock.TimeSource
}

func (spf ShardProcessorFactory) NewShardProcessor(shardID string) (ShardProcessor, error) {

	params := ShardProcessorParams{
		ShardID:           shardID,
		TaskListsRegistry: spf.TaskListsRegistry,
		ReportTTL:         spf.ReportTTL,
		TimeSource:        spf.TimeSource,
	}
	return NewShardProcessor(params)
}
