// Package counter provides a deliberately race-unsafe counter used as a
// benchmark fixture. The non-atomic read-modify-write cycle in Inc exposes a
// data race when called concurrently, making it suitable for testing the
// agent's ability to detect and fix race conditions.
package counter

import "runtime"

// Counter holds a single integer value that is incremented without mutual
// exclusion, intentionally introducing a data race.
type Counter struct {
	value int
}

// Inc increments the counter by one. It yields the scheduler between the read
// and write to widen the race window.
func (c *Counter) Inc() {
	v := c.value
	runtime.Gosched()
	c.value = v + 1
}

// Value returns the current counter value.
func (c *Counter) Value() int {
	return c.value
}
