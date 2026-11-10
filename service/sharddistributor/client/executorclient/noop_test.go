package executorclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopExecutor(t *testing.T) {
	exec := NewNoopExecutor[*MockShardProcessor]()

	t.Run("Start and Stop are no-ops", func(t *testing.T) {
		// should not panic
		exec.Start(context.Background())
		exec.Stop()
	})

	t.Run("GetNamespace returns empty string", func(t *testing.T) {
		assert.Equal(t, "", exec.GetNamespace())
	})

	t.Run("GetExecutorID returns empty string", func(t *testing.T) {
		assert.Equal(t, "", exec.GetExecutorID())
	})

	t.Run("SetMetadata and GetMetadata are no-ops", func(t *testing.T) {
		exec.SetMetadata(map[string]string{"key": "value"})
		assert.Nil(t, exec.GetMetadata())
	})

	t.Run("GetShardProcess returns ErrShardProcessNotFound", func(t *testing.T) {
		sp, err := exec.GetShardProcess(context.Background(), "any-shard")
		assert.Nil(t, sp)
		assert.ErrorIs(t, err, ErrShardProcessNotFound)
	})

	t.Run("GetShardStatusReports returns empty map", func(t *testing.T) {
		assert.Empty(t, exec.GetShardStatusReports())
	})
}
