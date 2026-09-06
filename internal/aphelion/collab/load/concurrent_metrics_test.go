package load

import (
	"testing"
	"time"
)

func TestConcurrentLatencySeparatesOfferSendAndApplication(t *testing.T) {
	scenario, err := GenerateConcurrent(ConcurrentConfig{Config: Config{Seed: 1, Clients: 2, Operations: 2, MaxX: 1, MaxY: 1}, ConflictPairs: 1})
	if err != nil {
		t.Fatal(err)
	}
	epoch := time.Unix(0, 0)
	m := newConcurrentMeasurements(scenario, epoch)
	m.records[0] = operationObservation{start: epoch.Add(50 * time.Millisecond), sent: epoch.Add(60 * time.Millisecond), acknowledged: epoch.Add(100 * time.Millisecond), allApplied: epoch.Add(150 * time.Millisecond), accepted: true}
	m.records[0].applied.add(0)
	m.records[0].applied.add(1)
	m.records[1] = operationObservation{start: epoch.Add(10 * time.Millisecond), sent: epoch.Add(20 * time.Millisecond), rejected: epoch.Add(30 * time.Millisecond)}
	r := m.result(epoch.Add(time.Second))
	if r.ScheduleToAcknowledgement.P50Millis != 100 || r.SendToAcknowledgement.P50Millis != 50 || r.SendToAllApplied.P50Millis != 100 || r.SendToRejection.P50Millis != 20 {
		t.Fatalf("latency boundaries collapsed: %#v", r)
	}
	if r.AppliedDeliveries != 2 || r.AcceptedOperations != 1 || r.RejectedOperations != 1 || r.PeakSendBacklog != 2 {
		t.Fatalf("incorrect actual counts: %#v", r)
	}
	summary := summarizeLatencies([]time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond})
	if summary.Count != 3 || summary.P50Millis != 20 || summary.P95Millis != 30 || summary.P99Millis != 30 {
		t.Fatal("percentiles do not use nearest rank")
	}
}

func TestConcurrentOfferScheduleDoesNotDriftWithSendDelay(t *testing.T) {
	epoch := time.Unix(0, 0)
	if scheduledAt(epoch, 8, 10) != epoch.Add(800*time.Millisecond) {
		t.Fatal("offer time depends on prior completion")
	}
	if scheduledCount(epoch, epoch.Add(450*time.Millisecond), 10, 20) != 5 || scheduledCount(epoch, epoch.Add(-time.Millisecond), 10, 20) != 0 || scheduledCount(epoch, epoch.Add(time.Minute), 10, 20) != 20 {
		t.Fatal("offered/backlog counts differ from absolute schedule")
	}
}

func TestConcurrentCompletionWaitsForSuccessfulSendAndEveryClient(t *testing.T) {
	scenario, err := GenerateConcurrent(ConcurrentConfig{Config: Config{Seed: 1, Clients: 2, Operations: 2, MaxX: 1, MaxY: 1}, ConflictPairs: 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m := newConcurrentMeasurements(scenario, now)
	// Readers can finish before the sending goroutine records Write's return.
	m.records[0] = operationObservation{start: now, acknowledged: now, accepted: true}
	m.records[0].applied.add(0)
	m.updateCompletionLocked(0)
	m.records[1] = operationObservation{start: now, sent: now, rejected: now}
	m.updateCompletionLocked(1)
	if m.complete() {
		t.Fatal("completed before a successful send and remote application")
	}
	m.records[0].applied.add(1)
	m.updateCompletionLocked(0)
	if m.complete() {
		t.Fatal("completed before the sender recorded success")
	}
	m.records[0].sent = now
	m.updateCompletionLocked(0)
	m.updateCompletionLocked(0)
	m.updateCompletionLocked(1)
	if !m.complete() {
		t.Fatal("valid completion or repeated observation was lost")
	}
}
