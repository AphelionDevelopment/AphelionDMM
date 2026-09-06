package load

import (
	"math"
	"math/bits"
	"sort"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

type LatencySummary struct {
	Count     int     `json:"count"`
	P50Millis float64 `json:"p50_ms"`
	P95Millis float64 `json:"p95_ms"`
	P99Millis float64 `json:"p99_ms"`
	MaxMillis float64 `json:"max_ms"`
}

type ClientConvergence struct {
	Client            int            `json:"client"`
	AppliedOperations int            `json:"applied_operations"`
	Revision          model.Revision `json:"revision"`
	MapHash           string         `json:"map_hash"`
}

type ConcurrentResult struct {
	Mode                         string              `json:"mode"`
	Seed                         int64               `json:"seed"`
	PlannedOperations            int                 `json:"planned_operations"`
	ScheduledOperations          int                 `json:"scheduled_operations"`
	StartedOperations            int                 `json:"started_operations"`
	SentOperations               int                 `json:"sent_operations"`
	AcceptedOperations           int                 `json:"accepted_operations"`
	RejectedOperations           int                 `json:"rejected_operations"`
	AppliedDeliveries            int                 `json:"applied_deliveries"`
	UnsentBacklog                int                 `json:"unsent_backlog"`
	UnresolvedOperations         int                 `json:"unresolved_operations"`
	PeakSendBacklog              int                 `json:"peak_send_backlog"`
	TargetOperationsPerSecond    int                 `json:"target_operations_per_second"`
	AchievedAcceptedPerSecond    float64             `json:"achieved_accepted_per_second"`
	OperationWindowMillis        float64             `json:"operation_window_ms"`
	ElapsedMillis                float64             `json:"elapsed_ms"`
	SendToAcknowledgement        LatencySummary      `json:"send_to_acknowledgement"`
	ScheduleToAcknowledgement    LatencySummary      `json:"schedule_to_acknowledgement"`
	SendToAllApplied             LatencySummary      `json:"send_to_all_applied"`
	SendToRejection              LatencySummary      `json:"send_to_rejection"`
	PresencePlanned              int                 `json:"presence_planned"`
	PresenceSent                 int                 `json:"presence_sent"`
	PresenceCoalesced            int                 `json:"presence_locally_coalesced"`
	PresenceObservedDeliveries   int                 `json:"presence_observed_deliveries"`
	PresenceUnobservedDeliveries int                 `json:"presence_unobserved_deliveries"`
	Clients                      []ClientConvergence `json:"clients"`
	ServerRevision               model.Revision      `json:"server_revision"`
	ServerMapHash                string              `json:"server_map_hash"`
	GatePassed                   bool                `json:"gate_passed"`
	TimedOut                     bool                `json:"timed_out"`
	Failure                      string              `json:"failure,omitempty"`
}

type clientMask [2]uint64 // Config bounds clients to 100.
func (mask *clientMask) add(client int) bool {
	bit := uint64(1) << uint(client%64)
	if mask[client/64]&bit != 0 {
		return false
	}
	mask[client/64] |= bit
	return true
}
func (mask clientMask) count() int { return bits.OnesCount64(mask[0]) + bits.OnesCount64(mask[1]) }

type operationObservation struct {
	start, sent, acknowledged, rejected, allApplied, latestApplied time.Time
	accepted                                                       bool
	completed                                                      bool
	applied                                                        clientMask
}

type concurrentMeasurements struct {
	mu                sync.Mutex
	sessionID         string
	scenario          ConcurrentScenario
	epoch             time.Time
	records           []operationObservation
	indexes           map[model.OperationID]int
	clients           []ClientConvergence
	presenceSent      [][]bool
	presenceSeen      [][]clientMask
	presenceCoalesced int
	completed         int
	err               error
}

func newConcurrentMeasurements(scenario ConcurrentScenario, epoch time.Time) *concurrentMeasurements {
	m := &concurrentMeasurements{scenario: scenario, epoch: epoch, records: make([]operationObservation, len(scenario.Intents)), indexes: make(map[model.OperationID]int, len(scenario.Intents))}
	hash, _ := scenario.Initial.Hash() // Verified before connection setup.
	for i, intent := range scenario.Intents {
		m.indexes[intent.Operation.OperationID] = i
	}
	for i := range scenario.Actors {
		m.clients = append(m.clients, ClientConvergence{Client: i, Revision: scenario.Initial.Revision, MapHash: hash})
		m.presenceSent = append(m.presenceSent, make([]bool, scenario.Config.PresencePerClient))
		m.presenceSeen = append(m.presenceSeen, make([]clientMask, scenario.Config.PresencePerClient))
	}
	return m
}

func scheduledCount(epoch, now time.Time, rate, count int) int {
	if now.Before(epoch) {
		return 0
	}
	if rate == 0 {
		return count
	}
	return min(count, int(now.Sub(epoch)/time.Second)*rate+int((now.Sub(epoch)%time.Second)*time.Duration(rate)/time.Second)+1)
}

func scheduledAt(epoch time.Time, index, rate int) time.Time {
	if rate == 0 {
		return epoch
	}
	return epoch.Add(time.Duration(index) * time.Second / time.Duration(rate))
}

func summarizeLatencies(values []time.Duration) LatencySummary {
	if len(values) == 0 {
		return LatencySummary{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	percentile := func(p float64) float64 {
		return float64(values[max(0, int(math.Ceil(p*float64(len(values))))-1)]) / float64(time.Millisecond)
	}
	return LatencySummary{Count: len(values), P50Millis: percentile(.50), P95Millis: percentile(.95), P99Millis: percentile(.99), MaxMillis: float64(values[len(values)-1]) / float64(time.Millisecond)}
}

func (m *concurrentMeasurements) complete() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completed == len(m.records)
}

// Called with mu held after send or outcome observations. Keep completion O(1)
// per event; scanning the whole manifest on every delivery distorts large runs.
func (m *concurrentMeasurements) updateCompletionLocked(index int) {
	record := &m.records[index]
	if !record.completed && !record.sent.IsZero() && (!record.rejected.IsZero() || (!record.acknowledged.IsZero() && record.applied.count() == len(m.clients))) {
		record.completed = true
		m.completed++
	}
}

func (m *concurrentMeasurements) result(now time.Time) ConcurrentResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := ConcurrentResult{Mode: "concurrent-independent-v1", Seed: m.scenario.Config.Seed, PlannedOperations: len(m.records), TargetOperationsPerSecond: m.scenario.Config.TargetOperationsPerSecond,
		ScheduledOperations: scheduledCount(m.epoch, now, m.scenario.Config.TargetOperationsPerSecond, len(m.records)),
		PresencePlanned:     len(m.clients) * m.scenario.Config.PresencePerClient, PresenceCoalesced: m.presenceCoalesced,
		Clients: append([]ClientConvergence(nil), m.clients...), ElapsedMillis: float64(max(time.Duration(0), now.Sub(m.epoch))) / float64(time.Millisecond)}
	var sendAck, scheduleAck, allApplied, rejected []time.Duration
	var sends []time.Time
	lastOutcome := m.epoch
	for i, record := range m.records {
		if !record.start.IsZero() {
			r.StartedOperations++
		}
		if !record.sent.IsZero() {
			r.SentOperations++
			sends = append(sends, record.sent)
		}
		if record.accepted {
			r.AcceptedOperations++
		}
		r.AppliedDeliveries += record.applied.count()
		if !record.acknowledged.IsZero() {
			sendAck = append(sendAck, record.acknowledged.Sub(record.start))
			scheduleAck = append(scheduleAck, record.acknowledged.Sub(scheduledAt(m.epoch, i, m.scenario.Config.TargetOperationsPerSecond)))
			if record.acknowledged.After(lastOutcome) {
				lastOutcome = record.acknowledged
			}
		}
		if !record.allApplied.IsZero() {
			allApplied = append(allApplied, record.allApplied.Sub(record.start))
		}
		if !record.rejected.IsZero() {
			r.RejectedOperations++
			rejected = append(rejected, record.rejected.Sub(record.start))
			if record.rejected.After(lastOutcome) {
				lastOutcome = record.rejected
			}
		}
	}
	sort.Slice(sends, func(i, j int) bool { return sends[i].Before(sends[j]) })
	for i, sent := range sends {
		r.PeakSendBacklog = max(r.PeakSendBacklog, scheduledCount(m.epoch, sent, r.TargetOperationsPerSecond, len(m.records))-i)
	}
	r.UnsentBacklog = max(0, r.ScheduledOperations-r.SentOperations)
	r.PeakSendBacklog = max(r.PeakSendBacklog, r.UnsentBacklog)
	r.UnresolvedOperations = r.StartedOperations - len(sendAck) - len(rejected)
	r.OperationWindowMillis = float64(lastOutcome.Sub(m.epoch)) / float64(time.Millisecond)
	if lastOutcome.After(m.epoch) {
		r.AchievedAcceptedPerSecond = float64(r.AcceptedOperations) / lastOutcome.Sub(m.epoch).Seconds()
	}
	r.SendToAcknowledgement, r.ScheduleToAcknowledgement, r.SendToAllApplied, r.SendToRejection = summarizeLatencies(sendAck), summarizeLatencies(scheduleAck), summarizeLatencies(allApplied), summarizeLatencies(rejected)
	for actor, sent := range m.presenceSent {
		for sequence, ok := range sent {
			if ok {
				r.PresenceSent++
				r.PresenceObservedDeliveries += m.presenceSeen[actor][sequence].count()
			}
		}
	}
	r.PresenceUnobservedDeliveries = r.PresenceSent*len(m.clients) - r.PresenceObservedDeliveries
	return r
}
