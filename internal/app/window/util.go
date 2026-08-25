package window

import "sync"

import "github.com/rs/zerolog/log"

func (w *Window) AddMouseChangeCallback(cb func(uint, uint)) (callbackId int) {
	id := w.mouseChangeCallbackId
	w.mouseChangeCallbacks[id] = cb
	w.mouseChangeCallbackId++
	log.Print("mouse change callback added:", id)
	return id
}

func (w *Window) RemoveMouseChangeCallback(id int) {
	delete(w.mouseChangeCallbacks, id)
	log.Print("mouse change callback deleted:", id)
}

var laterJobs []func()

// APHELION EDIT ADDITION START - COLLABORATION
var laterJobsMutex sync.Mutex

// APHELION EDIT ADDITION END

// RunLater queues provided a job to be run in the next frame.
func RunLater(job func()) {
	// APHELION EDIT ADDITION START - COLLABORATION
	laterJobsMutex.Lock()
	defer laterJobsMutex.Unlock()
	// APHELION EDIT ADDITION END
	laterJobs = append(laterJobs, job)
}

var repeatJobs []func()

// RunRepeat stores provided a job in a separate slice to run it in all other frames.
// Unlike the RunLater job will be executed all the time until the application shut down.
func RunRepeat(job func()) {
	repeatJobs = append(repeatJobs, job)
}
