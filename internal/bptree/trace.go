package bptree

// Event describes a single step in an operation, useful for visualization.
type Event struct {
	Type        string                 `json:"type"`
	Label       string                 `json:"label,omitempty"`
	Notes       string                 `json:"notes,omitempty"`
	EventPhase  string                 `json:"eventPhase,omitempty"`
	OperationID string                 `json:"operationId,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

func (t *Tree) record(eventType string, details map[string]interface{}) {
	t.trace = append(t.trace, Event{Type: eventType, Details: details})
}

func (t *Tree) recordLabeled(eventType, label, notes, phase string, details map[string]interface{}) {
	t.trace = append(t.trace, Event{
		Type:        eventType,
		Label:       label,
		Notes:       notes,
		EventPhase:  phase,
		Details:     details,
		OperationID: "",
	})
}

// resetTrace clears the trace buffer at the start of an operation.
func (t *Tree) resetTrace() {
	t.trace = t.trace[:0]
}

// Trace returns the events recorded by the most recent operation.
func (t *Tree) Trace() []Event {
	out := make([]Event, len(t.trace))
	copy(out, t.trace)
	return out
}
