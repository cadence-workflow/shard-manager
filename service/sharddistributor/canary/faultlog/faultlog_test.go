package faultlog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/latencykind"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

// Shard IDs picked so that latencykind.ShardIDToKind returns the named kind.
const (
	stuckStartShard = "68"
	stuckStopShard  = "6"
	normalShard     = "0"
)

func TestWrapLogger(t *testing.T) {
	// Guard the fixtures above: the hash, not this file, is the source of truth.
	require.Equal(t, latencykind.StuckStart, latencykind.ShardIDToKind(stuckStartShard))
	require.Equal(t, latencykind.StuckStop, latencykind.ShardIDToKind(stuckStopShard))
	require.Equal(t, latencykind.Normal, latencykind.ShardIDToKind(normalShard))

	tests := []struct {
		name          string
		message       string
		fields        []zap.Field
		wantLevel     zapcore.Level
		wantLifecycle string
		wantKind      string
	}{
		{
			name:          "injected start timeout is downgraded and tagged",
			message:       executorclient.MsgProcessorStartTimedOut,
			fields:        []zap.Field{zap.String(tag.ShardKey, stuckStartShard)},
			wantLevel:     zapcore.InfoLevel,
			wantLifecycle: "start",
			wantKind:      "stuck_start",
		},
		{
			name:          "injected stop timeout is downgraded and tagged",
			message:       executorclient.MsgProcessorStopTimedOut,
			fields:        []zap.Field{zap.String(tag.ShardKey, stuckStopShard)},
			wantLevel:     zapcore.InfoLevel,
			wantLifecycle: "stop",
			wantKind:      "stuck_stop",
		},
		{
			name:      "identical timeout on an uninjected shard stays an error",
			message:   executorclient.MsgProcessorStartTimedOut,
			fields:    []zap.Field{zap.String(tag.ShardKey, normalShard)},
			wantLevel: zapcore.ErrorLevel,
		},
		{
			name:      "start timeout on a stop-injected shard stays an error",
			message:   executorclient.MsgProcessorStartTimedOut,
			fields:    []zap.Field{zap.String(tag.ShardKey, stuckStopShard)},
			wantLevel: zapcore.ErrorLevel,
		},
		{
			name:      "stop timeout on a start-injected shard stays an error",
			message:   executorclient.MsgProcessorStopTimedOut,
			fields:    []zap.Field{zap.String(tag.ShardKey, stuckStartShard)},
			wantLevel: zapcore.ErrorLevel,
		},
		{
			name:      "timeout without a shard key stays an error",
			message:   executorclient.MsgProcessorStartTimedOut,
			wantLevel: zapcore.ErrorLevel,
		},
		{
			name:      "unrelated health signal on an injected shard stays an error",
			message:   "Failed to ping shard",
			fields:    []zap.Field{zap.String(tag.ShardKey, stuckStartShard)},
			wantLevel: zapcore.ErrorLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observedCore, logs := observer.New(zapcore.DebugLevel)
			WrapLogger(zap.New(observedCore)).Error(tt.message, tt.fields...)

			require.Equal(t, 1, logs.Len())
			entry := logs.All()[0]
			assert.Equal(t, tt.wantLevel, entry.Level)
			assert.Equal(t, tt.message, entry.Message)

			if tt.wantLevel == zapcore.ErrorLevel {
				assert.NotContains(t, entry.ContextMap(), "failure_injected")
				return
			}
			assert.Equal(t, map[string]interface{}{
				tag.ShardKey:       tt.fields[0].String,
				"canary":           true,
				"failure_injected": true,
				"expected":         true,
				"lifecycle":        tt.wantLifecycle,
				"latency_kind":     tt.wantKind,
			}, entry.ContextMap())
		})
	}
}

// Fields bound with With must survive the wrapper, and must not be mistaken for
// call-site fields when deciding whether a timeout was injected.
func TestWrapLoggerKeepsBoundFields(t *testing.T) {
	observedCore, logs := observer.New(zapcore.DebugLevel)
	logger := WrapLogger(zap.New(observedCore)).With(zap.String("executor_id", "executor-1"))

	logger.Error(executorclient.MsgProcessorStartTimedOut, zap.String(tag.ShardKey, stuckStartShard))

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	assert.Equal(t, "executor-1", entry.ContextMap()["executor_id"])
	assert.Equal(t, true, entry.ContextMap()["failure_injected"])
}
