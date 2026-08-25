package telemetry

import "go.opentelemetry.io/otel/metric"

type instruments struct {
	operations       metric.Int64Counter
	storeOperations  metric.Int64Counter
	replayOperations metric.Int64Counter
	presenceDropped  metric.Int64Counter
	connections      metric.Int64UpDownCounter
}

func newInstruments(meter metric.Meter) (instruments, error) {
	operations, err := meter.Int64Counter("aphelion.collab.operations", metric.WithUnit("{operation}"))
	if err != nil {
		return instruments{}, err
	}
	storeOperations, err := meter.Int64Counter("aphelion.collab.store.operations", metric.WithUnit("{operation}"))
	if err != nil {
		return instruments{}, err
	}
	replayOperations, err := meter.Int64Counter("aphelion.collab.replay.operations", metric.WithUnit("{operation}"))
	if err != nil {
		return instruments{}, err
	}
	presenceDropped, err := meter.Int64Counter("aphelion.collab.presence.dropped", metric.WithUnit("{update}"))
	if err != nil {
		return instruments{}, err
	}
	connections, err := meter.Int64UpDownCounter("aphelion.collab.connections", metric.WithUnit("{connection}"))
	if err != nil {
		return instruments{}, err
	}
	return instruments{
		operations:       operations,
		storeOperations:  storeOperations,
		replayOperations: replayOperations,
		presenceDropped:  presenceDropped,
		connections:      connections,
	}, nil
}
