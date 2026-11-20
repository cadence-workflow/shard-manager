package dynamicconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log"
)

func BenchmarkGetIntProperty(b *testing.B) {
	client := NewInMemoryClient()
	cln := NewCollection(client, log.NewNoop())
	key := dynamicproperties.TestGetIntPropertyKey
	for i := 0; i < b.N; i++ {
		size := cln.GetIntProperty(key)
		assert.Equal(b, 0, size())
	}
}
