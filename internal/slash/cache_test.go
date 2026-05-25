package slash

import (
	"sync"
	"testing"
)

func TestCache_GetMissThenHit(t *testing.T) {
	c := NewCache()
	if _, ok := c.Get("k"); ok {
		t.Error("expected miss on fresh cache")
	}
	cmds := []*Command{{Name: "x"}}
	c.Set("k", cmds)
	got, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if len(got) != 1 || got[0].Name != "x" {
		t.Errorf("Get returned %+v", got)
	}
}

func TestCache_Invalidate(t *testing.T) {
	c := NewCache()
	c.Set("a", []*Command{{Name: "a"}})
	c.Set("b", []*Command{{Name: "b"}})
	c.Invalidate()
	if _, ok := c.Get("a"); ok {
		t.Error("a should be invalidated")
	}
	if _, ok := c.Get("b"); ok {
		t.Error("b should be invalidated")
	}
}

func TestCache_InvalidateKey(t *testing.T) {
	c := NewCache()
	c.Set("a", []*Command{{Name: "a"}})
	c.Set("b", []*Command{{Name: "b"}})
	c.InvalidateKey("a")
	if _, ok := c.Get("a"); ok {
		t.Error("a should be invalidated")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("b should still be cached")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set("k", []*Command{{Name: "x"}})
			_, _ = c.Get("k")
		}()
	}
	wg.Wait()
}
