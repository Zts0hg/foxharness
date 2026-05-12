package counter

import "runtime"

type Counter struct {
	value int
}

func (c *Counter) Inc() {
	v := c.value
	runtime.Gosched()
	c.value = v + 1
}

func (c *Counter) Value() int {
	return c.value
}
