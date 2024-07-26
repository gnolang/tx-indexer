package queue

// Item represents a single queue item
type Item interface {
	// Less returns a flag indicating if the element
	// has a lower value than another element.
	// For max-priority queue implementations, Less should return true if A > B
	Less(b Item) bool
}
