// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uconfig "go.uber.org/config"
	"gopkg.in/validator.v2"

	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/service"
)

func TestToString(t *testing.T) {
	var cfg Config
	err := Load("", "../../config", "", &cfg)
	assert.NoError(t, err)
	assert.NotEmpty(t, cfg.String())
}

func TestGetServiceConfig(t *testing.T) {
	cfg := Config{}
	_, err := cfg.GetServiceConfig(service.ShardDistributor)
	assert.EqualError(t, err, "no config section for service: sharddistributor")

	cfg = Config{Services: map[string]Service{"sharddistributor": {RPC: RPC{GRPCPort: 123}}}}
	svc, err := cfg.GetServiceConfig(service.ShardDistributor)
	assert.NoError(t, err)
	assert.NotEmpty(t, svc)
}

func TestHistogramMigrationConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		orig := metrics.HistogramMigrationMetrics
		t.Cleanup(func() {
			metrics.HistogramMigrationMetrics = orig
		})
		metrics.HistogramMigrationMetrics = map[string]struct{}{
			"key1": {},
			"key2": {},
			"key3": {},
			"key4": {},
		}

		// intentionally avoiding the full config struct, as it has other required config
		yaml, err := uconfig.NewYAML(uconfig.RawSource(strings.NewReader(`
default: histogram
names:
  key1: true
  key2: false
  key3:
`)))
		require.NoError(t, err)

		var cfg metrics.HistogramMigration
		err = yaml.Get(uconfig.Root).Populate(&cfg)
		require.NoError(t, err)

		err = validator.Validate(cfg)
		require.NoError(t, err)

		check := func(key string, timer, histogram bool) {
			assert.Equalf(t, timer, cfg.EmitTimer(key), "wrong value for EmitTimer(%q)", key)
			assert.Equalf(t, histogram, cfg.EmitHistogram(key), "wrong value for EmitHistogram(%q)", key)
		}
		check("key1", true, true)
		check("key2", false, false)
		check("key3", false, false) // the type's default mode == false.  not truly intended behavior, but it's weird config so it's fine.
		check("key4", false, true)  // configured default == histogram
		check("key5", true, true)   // not migrating = always emitted
		if t.Failed() {
			t.Logf("config: %#v", cfg)
		}
	})
	t.Run("invalid default", func(t *testing.T) {
		yaml, err := uconfig.NewYAML(uconfig.RawSource(strings.NewReader(`
default: xyz
`)))
		require.NoError(t, err)

		var cfg metrics.HistogramMigration
		err = yaml.Get(uconfig.Root).Populate(&cfg)
		assert.ErrorContains(t, err, `unsupported histogram migration mode "xyz", must be "timer", "histogram", or "both"`)
	})
	t.Run("invalid key", func(t *testing.T) {
		orig := metrics.HistogramMigrationMetrics
		t.Cleanup(func() {
			metrics.HistogramMigrationMetrics = orig
		})
		metrics.HistogramMigrationMetrics = map[string]struct{}{
			"key1": {},
		}
		yaml, err := uconfig.NewYAML(uconfig.RawSource(strings.NewReader(`
names:
  key1: xyz
`)))
		require.NoError(t, err)

		var cfg metrics.HistogramMigration
		err = yaml.Get(uconfig.Root).Populate(&cfg)
		assert.ErrorContains(t, err, "cannot unmarshal !!str `xyz` into bool")
	})
	t.Run("nonexistent key", func(t *testing.T) {
		yaml, err := uconfig.NewYAML(uconfig.RawSource(strings.NewReader(`
names:
  definitely_does_not_exist: true
`)))
		require.NoError(t, err)

		var cfg metrics.HistogramMigration
		err = yaml.Get(uconfig.Root).Populate(&cfg)
		assert.ErrorContains(t, err, `unknown histogram-migration metric name "definitely_does_not_exist"`)
	})
}
