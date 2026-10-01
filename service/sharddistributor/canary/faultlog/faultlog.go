// Package faultlog keeps the canary's deliberately stalled shards out of the
// executor framework's ERROR logs.
//
// The canary parks Start/Stop past the executor's async operation timeout on a
// small fraction of shards (see package latencykind) so the framework's timeout
// path is exercised in production. The framework cannot tell a deliberate stall
// from a real one, so it logs every timeout at ERROR. This core wraps only the
// logger the canary hands its own executors and downgrades exactly the timeouts
// it can attribute to its own injection; every other record, and every logger
// outside those executors, is left alone.
package faultlog

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/latencykind"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon/tag"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

// WrapLogger returns a logger that reports the canary's deliberately stalled shards
// timeouts at INFO instead of ERROR.
func WrapLogger(logger *zap.Logger) *zap.Logger {
	return logger.WithOptions(zap.WrapCore(func(inner zapcore.Core) zapcore.Core {
		return &injectedTimeoutCore{Core: inner}
	}))
}

type injectedTimeoutCore struct {
	zapcore.Core
}

// We add ourselves as a core to the entry so Write is called when the entry is written.
func (c *injectedTimeoutCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *injectedTimeoutCore) With(fields []zapcore.Field) zapcore.Core {
	return &injectedTimeoutCore{Core: c.Core.With(fields)}
}

func (c *injectedTimeoutCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if kind, lifecycle, ok := c.injected(ent, fields); ok {
		ent.Level = zapcore.InfoLevel
		// Copy on append: the caller owns the backing array.
		fields = append(fields[:len(fields):len(fields)],
			zap.Bool("canary", true),
			zap.Bool("failure_injected", true),
			zap.Bool("expected", true),
			zap.String("lifecycle", lifecycle),
			zap.String("latency_kind", kind.String()),
		)
	}
	return c.Core.Write(ent, fields)
}

// injected reports whether ent is the framework timing out a shard the canary stalled on purpose.
func (c *injectedTimeoutCore) injected(ent zapcore.Entry, fields []zapcore.Field) (kind latencykind.Kind, lifecycle string, ok bool) {
	var want latencykind.Kind
	switch ent.Message {
	case executorclient.MsgProcessorStartTimedOut:
		want, lifecycle = latencykind.StuckStart, "start"
	case executorclient.MsgProcessorStopTimedOut:
		want, lifecycle = latencykind.StuckStop, "stop"
	default:
		return latencykind.Normal, "", false
	}

	// Make sure this is the timeout we expect from the shard
	if latencykind.ShardIDToKind(shardKey(fields)) != want {
		return latencykind.Normal, "", false
	}
	return want, lifecycle, true
}

// shardKey returns the record's shard ID, or "" when it has none.
func shardKey(fields []zapcore.Field) string {
	for _, f := range fields {
		if f.Key == tag.ShardKey {
			return f.String
		}
	}
	return ""
}
