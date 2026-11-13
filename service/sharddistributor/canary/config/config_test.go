package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*LoadConfig)
		wantErr string
	}{
		{name: "valid", modify: func(*LoadConfig) {}},
		{name: "no namespace", modify: func(c *LoadConfig) { c.Namespace = "" }, wantErr: "namespace must be set"},
		{name: "zero interval", modify: func(c *LoadConfig) { c.Interval = 0 }, wantErr: "interval must be positive"},
		{name: "no baseline executors", modify: func(c *LoadConfig) { c.BaselineExecutors = 0 }, wantErr: "number of baseline executors must be positive"},
		{name: "no executors", modify: func(c *LoadConfig) { c.NumExecutors = 0 }, wantErr: "number of executors must be positive"},
		{name: "zero rate", modify: func(c *LoadConfig) { c.ExecutorsPerSecond = 0 }, wantErr: "executors per second must be in (0, 100]"},
		{name: "rate above max", modify: func(c *LoadConfig) { c.ExecutorsPerSecond = maxLoadExecutorsPerSecond + 1 }, wantErr: "executors per second must be in (0, 100]"},
		{name: "negative hold", modify: func(c *LoadConfig) { c.Hold = -time.Second }, wantErr: "hold must not be negative"},
		{name: "zero convergence timeout", modify: func(c *LoadConfig) { c.ConvergenceTimeout = 0 }, wantErr: "convergence timeout must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoadConfig{
				Namespace:          "test-load",
				Interval:           time.Hour,
				BaselineExecutors:  1,
				NumExecutors:       2,
				ExecutorsPerSecond: 10,
				Hold:               10 * time.Second,
				ConvergenceTimeout: 10 * time.Second,
			}
			tt.modify(&cfg)

			err := cfg.Validate()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
