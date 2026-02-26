package example

import (
	"context"
	"fmt"
	"time"
)

type Tracer struct {
	StartTime int64
}

func StartTracing(ctx context.Context, name string) (context.Context, Tracer) {
	return ctx, Tracer{
		StartTime: time.Now().Unix(),
	}
}

func (t Tracer) End() {
	// You can calculate the duration here if needed
	duration := time.Now().Unix() - t.StartTime
	fmt.Printf("Tracing ended. Duration: %d seconds\n", duration)
}
