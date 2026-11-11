package testlogger

import (
	"testing"

	"go.uber.org/fx"
)

// Module allows to push testlogger for tests.
func Module(t *testing.T) fx.Option {
	return fx.Options(
		fx.Provide(func() TestingT { return TestingT(t) }),
		fx.Provide(New),
	)
}
