package otel

import (
	"context"

	"github.com/nnstd/gun/runtime/eventloop"
)

// Enabled is true when compiled with -tags otel.
var Enabled = false

func SetActiveContext(ctx context.Context) {
	eventloop.Default.SetActiveContext(ctx)
}

func ActiveContext() context.Context {
	return eventloop.Default.ActiveContext()
}

