package manager

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/cache"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/namespaces"
	"github.com/cadence-workflow/shard-manager/service/sharddistributor/store"
)

func TestNew(t *testing.T) {
	cfg := config.ShardDistribution{Namespaces: []config.Namespace{
		{Name: "fixed", Type: config.NamespaceTypeFixed, ShardNum: 1},
		{Name: "ephemeral", Type: config.NamespaceTypeEphemeral},
	}}

	mockStore := store.NewMockStore(gomock.NewController(t))
	for _, ns := range cfg.Namespaces {
		mockStore.EXPECT().SubscribeToNamespaceChanges(ns.Name).Return((<-chan struct{})(make(chan struct{})), nil)
	}

	lifecycle := fxtest.NewLifecycle(t)
	m := New(Params{
		Registry:      namespaces.MustNewRegistry(cfg),
		Store:         mockStore,
		Config:        &config.Config{},
		Logger:        testlogger.New(t),
		TimeSource:    clock.NewRealTimeSource(),
		MetricsClient: metrics.NewNoopMetricsClient(),
		Lifecycle:     lifecycle,
	})
	lifecycle.RequireStart()
	defer lifecycle.RequireStop()

	tests := []struct {
		name         string
		namespace    string
		wantErr      error
		wantAssigner bool
	}{
		{name: "fixed has no assigner", namespace: "fixed"},
		{name: "ephemeral has an assigner", namespace: "ephemeral", wantAssigner: true},
		{name: "unknown", namespace: "unknown", wantErr: &types.NamespaceNotFoundError{Namespace: "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := m.Get(tt.namespace)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.namespace, ns.Name())
			assert.NotNil(t, ns.Cache())
			assert.Equal(t, tt.wantAssigner, ns.Assigner() != nil)
		})
	}

	assert.Len(t, m.List(), 2)
}

func TestStartStop(t *testing.T) {
	tests := []struct {
		name     string
		expect   func(a, b, c *cache.MockShardCache)
		wantErr  string
		wantLogs []string
	}{
		{
			name: "stops in reverse order",
			expect: func(a, b, c *cache.MockShardCache) {
				gomock.InOrder(
					a.EXPECT().Start().Return(nil),
					b.EXPECT().Start().Return(nil),
					c.EXPECT().Start().Return(nil),
					c.EXPECT().Stop(),
					b.EXPECT().Stop(),
					a.EXPECT().Stop(),
				)
			},
			wantLogs: []string{
				"info: a Starting", "info: a Started",
				"info: b Starting", "info: b Started",
				"info: c Starting", "info: c Started",
				"info: c Stopping", "info: c Stopped",
				"info: b Stopping", "info: b Stopped",
				"info: a Stopping", "info: a Stopped",
			},
		},
		{
			name: "start failure stops only the started namespaces",
			expect: func(a, b, _ *cache.MockShardCache) {
				gomock.InOrder(
					a.EXPECT().Start().Return(nil),
					b.EXPECT().Start().Return(errors.New("boom")),
					a.EXPECT().Stop(),
				)
			},
			wantErr: `start cache of namespace "b": boom`,
			wantLogs: []string{
				"info: a Starting", "info: a Started",
				"info: b Starting", "error: b StartFailed",
				"info: a Stopping", "info: a Stopped",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			cacheA, cacheB, cacheC := cache.NewMockShardCache(ctrl), cache.NewMockShardCache(ctrl), cache.NewMockShardCache(ctrl)
			tt.expect(cacheA, cacheB, cacheC)
			logger, logs := testlogger.NewObserved(t)
			m := newManager(logger, []Namespace{
				newNamespace(config.Namespace{Name: "a"}, cacheA, nil),
				newNamespace(config.Namespace{Name: "b"}, cacheB, nil),
				newNamespace(config.Namespace{Name: "c"}, cacheC, nil),
			})

			if err := m.Start(); tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				m.Stop()
			}

			var gotLogs []string
			for _, entry := range logs.All() {
				fields := entry.ContextMap()
				gotLogs = append(gotLogs, fmt.Sprintf("%v: %v %v", entry.Level, fields["shard-namespace"], fields["lifecycle"]))
			}
			assert.Equal(t, tt.wantLogs, gotLogs)
		})
	}
}
