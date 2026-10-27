// Package contexttest provides contexts that tests end at a chosen point of a
// loop rather than at a time.
package contexttest

import "context"

// Countdown is a context that ends once its Err has been asked more than
// Budget times, with EndsWith as its error. It lets a test end a context in
// the middle of a loop that checks it, without a clock, and Asked tells how
// often the loop checked it. It is meant for one goroutine.
type Countdown struct {
	context.Context
	EndsWith error
	Budget   int
	Asked    int
}

// Err counts the ask, and answers EndsWith once Budget asks are spent.
func (c *Countdown) Err() error {
	c.Asked++

	if c.Asked > c.Budget {
		return c.EndsWith
	}

	return nil
}
