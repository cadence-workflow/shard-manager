package clockfx

import (
	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/clock"
)

// Module provides real time source for fx application.
var Module = fx.Module("clock", fx.Provide(clock.NewRealTimeSource))
