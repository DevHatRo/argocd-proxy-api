package cache

import (
	"sync"
	"testing"
	"time"
)

func TestGetOnEmptyCache(t *testing.T) {
	c := New[string](30 * time.Second)

	val, ok := c.Get()
	if ok {
		t.Error("expected cache miss on empty cache")
	}
	if val != "" {
		t.Errorf("expected zero value, got %q", val)
	}
}

func TestSetAndGet(t *testing.T) {
	c := New[string](30 * time.Second)

	c.Set("hello")

	val, ok := c.Get()
	if !ok {
		t.Error("expected cache hit after Set")
	}
	if val != "hello" {
		t.Errorf("expected %q, got %q", "hello", val)
	}
}

func TestExpiry(t *testing.T) {
	c := New[int](50 * time.Millisecond)

	c.Set(42)

	val, ok := c.Get()
	if !ok || val != 42 {
		t.Fatalf("expected hit with value 42, got ok=%v val=%v", ok, val)
	}

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get()
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestInvalidate(t *testing.T) {
	c := New[string](30 * time.Second)

	c.Set("data")
	c.Invalidate()

	_, ok := c.Get()
	if ok {
		t.Error("expected cache miss after Invalidate")
	}
}

func TestZeroTTLDisablesCaching(t *testing.T) {
	c := New[string](0)

	c.Set("should not cache")

	_, ok := c.Get()
	if ok {
		t.Error("expected cache miss when TTL is 0")
	}
}

func TestOverwrite(t *testing.T) {
	c := New[int](30 * time.Second)

	c.Set(1)
	c.Set(2)

	val, ok := c.Get()
	if !ok || val != 2 {
		t.Errorf("expected 2, got ok=%v val=%v", ok, val)
	}
}

func TestSliceCache(t *testing.T) {
	c := New[[]string](30 * time.Second)

	c.Set([]string{"a", "b", "c"})

	val, ok := c.Get()
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(val) != 3 || val[0] != "a" || val[2] != "c" {
		t.Errorf("unexpected cached slice: %v", val)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New[int](30 * time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(v int) {
			defer wg.Done()
			c.Set(v)
		}(i)
		go func() {
			defer wg.Done()
			c.Get()
		}()
	}
	wg.Wait()

	_, ok := c.Get()
	if !ok {
		t.Error("expected cache to have a value after concurrent writes")
	}
}
