package wand_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/headlesslab/wand"
	"github.com/ysmood/got"
)

// TestPoolGetAfterCleanup: a Get waiting for a slot, and every Get after it,
// returns ErrPoolCleanedUp once Cleanup ran, instead of waiting for a slot
// that never comes (rod #1117).
func TestPoolGetAfterCleanup(t *testing.T) {
	g := got.T(t)

	one := 1
	pool := wand.NewPool[int](1)
	get := func() (*int, error) { return pool.Get(func() (*int, error) { return &one, nil }) }

	held, err := get()
	g.E(err)

	var waiting sync.WaitGroup
	waiting.Add(1)
	var waitErr error
	go func() {
		defer waiting.Done()
		_, waitErr = get()
	}()

	pool.Cleanup(func(*int) {})
	waiting.Wait()
	g.True(errors.Is(waitErr, wand.ErrPoolCleanedUp))

	_, err = get()
	g.True(errors.Is(err, wand.ErrPoolCleanedUp))
	g.Eq(err.Error(), "pool has been cleaned up")
	g.Panic(func() { pool.MustGet(func() *int { return &one }) })

	pool.Put(held)
}

// TestPoolCleanup: Cleanup runs its function on every element the pool holds,
// an element out of the pool at the time goes to that function when it is
// Put back, from whichever goroutine and never overlapping Cleanup's own
// calls, a placeholder never does, and a second Cleanup finds nothing.
func TestPoolCleanup(t *testing.T) {
	g := got.T(t)

	pool := wand.NewPool[int](3)
	next := 0
	create := func() *int { next++; n := next; return &n }

	x, y, z := pool.MustGet(create), pool.MustGet(create), pool.MustGet(create)
	pool.Put(x)
	pool.Put(y)

	// The function appends with no lock of its own: the pool serializes it.
	cleaned := []int{}
	pool.Cleanup(func(n *int) { cleaned = append(cleaned, *n) })
	g.Eq(cleaned, []int{1, 2})

	var late sync.WaitGroup
	late.Add(1)
	go func() {
		defer late.Done()
		pool.Put(z)
	}()
	late.Wait()
	g.Eq(cleaned, []int{1, 2, 3})

	pool.Put(nil)
	g.Eq(cleaned, []int{1, 2, 3})

	pool.Cleanup(func(*int) { g.Fatal("a second Cleanup found an element") })
}

// TestPoolPutWithoutGet: a Put with every slot in the pool already is a Put
// without a Get, and panics rather than waiting for a slot that never frees.
func TestPoolPutWithoutGet(t *testing.T) {
	g := got.T(t)

	one := 1
	g.Panic(func() { wand.NewPool[int](1).Put(&one) })
}
