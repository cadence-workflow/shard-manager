package namespacemanager

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
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

func TestStartRollsBackOnFailure(t *testing.T) {
	var events []string
	newComponent := func(name string, startErr error) component {
		return component{
			name:  name,
			start: func() error { events = append(events, "start "+name); return startErr },
			stop:  func() { events = append(events, "stop "+name) },
		}
	}

	m := NewStatic()
	m.components = []component{
		newComponent("a", nil),
		newComponent("b", nil),
		newComponent("c", errors.New("boom")),
		newComponent("d", nil),
	}

	require.ErrorContains(t, m.start(context.Background()), "start c: boom")
	assert.Equal(t, []string{"start a", "start b", "start c", "stop b", "stop a"}, events)
}
