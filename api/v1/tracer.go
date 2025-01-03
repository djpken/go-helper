package v1

import (
	"github.com/djpken/go-helper/pkg/tracing"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer(tracing.Rest)
