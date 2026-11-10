package testlogger

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/cadence-workflow/shard-manager/common/log"
)

func TestModule(t *testing.T) {
	app := fxtest.New(t, Module(t), fx.Invoke(func(logger log.Logger) {}))
	app.RequireStart().RequireStop()
}
