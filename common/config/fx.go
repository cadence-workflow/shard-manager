package config

import (
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/common/metrics"
)

// Module returns a config.Provider that could be used byother components.
var Module = fx.Module("configfx",
	fx.Provide(New),
)

type Context struct {
	Environment string
	Zone        string
}

// Params defines the dependencies of the configfx module.
type Params struct {
	fx.In

	Context   Context
	LookupEnv LookupEnvFunc `optional:"true"`

	ConfigDir string `name:"config-dir"`

	Lifecycle fx.Lifecycle `optional:"true"` // required for strict mode
}

// Result defines the objects that the configfx module provides.
type Result struct {
	fx.Out

	Config        Config
	ServiceConfig Service
	MigrationCfg  metrics.MigrationConfig
}

// LookupEnvFunc returns the value of the environment variable given by key.
// It should behave the same as `os.LookupEnv`. If a function returns false,
// an environment variable is looked up using `os.LookupEnv`.
type LookupEnvFunc func(key string) (string, bool)

// New exports functionality similar to Module, but allows the caller to wrap
// or modify Result. Most users should use Module instead.
func New(p Params) (Result, error) {
	lookupFun := os.LookupEnv
	if p.LookupEnv != nil {
		lookupFun = func(key string) (string, bool) {
			if result, ok := p.LookupEnv(key); ok {
				return result, true
			}
			return lookupFun(key)
		}
	}

	var cfg Config
	err := Load(p.Context.Environment, p.ConfigDir, p.Context.Zone, &cfg)
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}

	cfg.fillDefaults()

	p.Lifecycle.Append(fx.StartHook(cfg.validate))

	return Result{
		Config:        cfg,
		ServiceConfig: cfg.ServiceConfig(),
		MigrationCfg: metrics.MigrationConfig{
			Histogram: cfg.Histograms,
			Gauge:     cfg.Gauges,
			Counter:   cfg.Counters,
		},
	}, nil
}
