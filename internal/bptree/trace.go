package bptree

// Event describes a single step in an operation, useful for visualization.
type Event struct {
	Type    string                 `json:"type"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (t *Tree) record(eventType string, details map[string]interface{}) {
	t.trace = append(t.trace, Event{Type: eventType, Details: details})
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
