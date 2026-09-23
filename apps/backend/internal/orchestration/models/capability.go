package models

// Capability describes one broker tool a coordinator may call. Effect is
// read or write.
type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Effect      string `json:"effect"`
}

type CapabilityPage struct {
	Entries    []Capability `json:"entries"`
	NextCursor string       `json:"next_cursor"`
}
