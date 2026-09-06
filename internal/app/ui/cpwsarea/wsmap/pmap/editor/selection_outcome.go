// APHELION EDIT ADDITION START - SELECTION HISTORY
package editor

// TrackSelectionTransform associates UI geometry with the operation committed
// inside action. Outcomes and history callbacks run on the UI thread and retain
// the attachment fence used by map state; geometry never crosses the protocol.
func (e *Editor) TrackSelectionTransform(changed func(bool), action func() error) error {
	previous := e.selectionOutcome
	e.selectionOutcome = changed
	defer func() { e.selectionOutcome = previous }()
	return action()
}

func selectionApplied(changed func(bool), applied bool) {
	if changed != nil {
		changed(applied)
	}
}

// APHELION EDIT ADDITION END
