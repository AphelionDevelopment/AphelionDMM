package window

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunLaterAcceptsConcurrentProducersWithoutLosingJobs(t *testing.T) {
	laterJobs = nil
	t.Cleanup(func() { laterJobs = nil })

	const producers = 16
	const jobsPerProducer = 64
	var producersDone sync.WaitGroup
	var completed atomic.Int64
	for range producers {
		producersDone.Add(1)
		go func() {
			defer producersDone.Done()
			for range jobsPerProducer {
				RunLater(func() { completed.Add(1) })
			}
		}()
	}
	producersDone.Wait()
	runLaterJobs()
	if actual := completed.Load(); actual != producers*jobsPerProducer {
		t.Fatalf("completed jobs = %d, want %d", actual, producers*jobsPerProducer)
	}
}
