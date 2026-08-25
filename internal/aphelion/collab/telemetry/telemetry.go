package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "sdmm/internal/aphelion/collab"

type OperationSignal string

const (
	OperationSubmit  OperationSignal = "submit"
	OperationInverse OperationSignal = "inverse"
)

type StoreSignal string

const (
	StoreCreate   StoreSignal = "create"
	StoreAppend   StoreSignal = "append"
	StoreSnapshot StoreSignal = "snapshot"
	StoreLoad     StoreSignal = "load"
)

type Config struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

type Telemetry struct {
	tracer  trace.Tracer
	metrics instruments
}

func New(config Config) (*Telemetry, error) {
	if config.TracerProvider == nil {
		config.TracerProvider = otel.GetTracerProvider()
	}
	if config.MeterProvider == nil {
		config.MeterProvider = otel.GetMeterProvider()
	}
	metrics, err := newInstruments(config.MeterProvider.Meter(instrumentationName))
	if err != nil {
		return nil, err
	}
	return &Telemetry{tracer: config.TracerProvider.Tracer(instrumentationName), metrics: metrics}, nil
}

func (telemetry *Telemetry) Operation(ctx context.Context, signal OperationSignal) (context.Context, func(error)) {
	ctx, span := telemetry.tracer.Start(ctx, "aphelion.collab.operation", trace.WithAttributes(attribute.String("operation.kind", string(signal))))
	return ctx, func(err error) {
		outcome := finishSpan(span, err)
		telemetry.metrics.operations.Add(ctx, 1, metric.WithAttributes(attribute.String("operation.kind", string(signal)), attribute.String("outcome", outcome)))
	}
}

func (telemetry *Telemetry) Store(ctx context.Context, signal StoreSignal) (context.Context, func(error)) {
	ctx, span := telemetry.tracer.Start(ctx, "aphelion.collab.store", trace.WithAttributes(attribute.String("store.operation", string(signal))))
	return ctx, func(err error) {
		outcome := finishSpan(span, err)
		telemetry.metrics.storeOperations.Add(ctx, 1, metric.WithAttributes(attribute.String("store.operation", string(signal)), attribute.String("outcome", outcome)))
	}
}

func (telemetry *Telemetry) Replay(ctx context.Context, operations int) (context.Context, func(error)) {
	ctx, span := telemetry.tracer.Start(ctx, "aphelion.collab.replay", trace.WithAttributes(attribute.Int("replay.operations", operations)))
	return ctx, func(err error) {
		outcome := finishSpan(span, err)
		telemetry.metrics.replayOperations.Add(ctx, int64(operations), metric.WithAttributes(attribute.String("outcome", outcome)))
	}
}

func (telemetry *Telemetry) PresenceDropped(ctx context.Context) {
	telemetry.metrics.presenceDropped.Add(ctx, 1)
}

func (telemetry *Telemetry) ConnectionChanged(ctx context.Context, delta int64) {
	telemetry.metrics.connections.Add(ctx, delta)
}

func finishSpan(span trace.Span, err error) string {
	if err != nil {
		span.SetStatus(codes.Error, "failed")
		span.SetAttributes(attribute.String("outcome", "error"))
		span.End()
		return "error"
	}
	span.SetStatus(codes.Ok, "")
	span.SetAttributes(attribute.String("outcome", "success"))
	span.End()
	return "success"
}
