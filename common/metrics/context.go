package metrics

import "context"

type contextTag string

const contextTagsKey = contextTag("metrics.Tags")

func TagContext(ctx context.Context, ctxTags ...Tag) context.Context {
	tags, ok := ctx.Value(contextTagsKey).([]Tag)
	if !ok {
		tags = []Tag{}
	}
	tags = append(tags, ctxTags...)
	return context.WithValue(ctx, contextTagsKey, tags)
}

func GetContextTags(ctx context.Context) []Tag {
	tags, ok := ctx.Value(contextTagsKey).([]Tag)
	if !ok {
		tags = []Tag{}
	}
	return tags
}
