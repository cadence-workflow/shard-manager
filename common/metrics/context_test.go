package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextTags(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, GetContextTags(ctx))

	tag1 := DomainTag("domain")
	ctx = TagContext(ctx, tag1)
	assert.Equal(t, []Tag{tag1}, GetContextTags(ctx))

	tag2 := TransportTag("grpc")
	ctx = TagContext(ctx, tag2)
	assert.Equal(t, []Tag{tag1, tag2}, GetContextTags(ctx))

	ctx1 := context.Background()
	ctx1 = TagContext(ctx1, tag1, tag2)
	assert.Contains(t, GetContextTags(ctx1), tag1)
	assert.Contains(t, GetContextTags(ctx1), tag2)
}
