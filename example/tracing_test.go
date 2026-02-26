package example_test

import (
	"context"
	"testing"

	"example"
)

func TestTracing(t *testing.T) {
	ctx := context.Background()
	ctx, tracer := example.StartTracing(ctx, "TestTracing")
	defer tracer.End()
}
