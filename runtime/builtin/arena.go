package jsvalue

import (
	"os"
	"sync"
)

const (
	// JSValue is ~112 bytes, so 256 slots is roughly ~28 KiB per chunk.
	// This is a better tradeoff than 1024 (~112 KiB per chunk):
	// lower retained heap, better cache locality, still low metadata overhead.
	arenaChunkSize = 256

	// Keep only a tiny warm cache of spare chunks per arena.
	maxSpareChunks = 2

	// If an arena grew too much during a spike, do not return it to the pool.
	// Let GC reclaim it instead of poisoning the pool.
	maxRetainedArenaChunks = 8

	// Avoid retaining very deep scope stacks forever.
	maxRetainedMarksCap = 64
)

type arenaChunk struct {
	slots [arenaChunkSize]JSValue
	next  *arenaChunk
}

type arenaMark struct {
	chunk *arenaChunk
	pos   int
}

type Arena struct {
	current *arenaChunk
	spare   *arenaChunk

	pos int

	marks []arenaMark

	// Number of chunks currently in spare list.
	spareCount int

	// Total number of chunks owned by this arena:
	// current chain + spare chain.
	chunkCount int
}

func NewArena() *Arena {
	return &Arena{}
}

func (a *Arena) reset() {
	if a == nil {
		return
	}

	a.pos = 0
	a.marks = a.marks[:0]
	if cap(a.marks) > maxRetainedMarksCap {
		a.marks = nil
	}

	if a.current == nil {
		return
	}

	// Keep current chunk attached as the active chunk.
	// Move older current-chain chunks into bounded spare cache.
	ch := a.current.next
	a.current.next = nil

	for ch != nil {
		next := ch.next
		ch.next = nil
		a.putSpare(ch)
		ch = next
	}
}

func (a *Arena) Destroy() {}

func (a *Arena) putSpare(ch *arenaChunk) {
	if ch == nil {
		return
	}

	// Drop extra chunks instead of retaining them forever.
	if a.spareCount >= maxSpareChunks {
		a.chunkCount--
		return
	}

	ch.next = a.spare
	a.spare = ch
	a.spareCount++
}

func (a *Arena) nextChunk() *arenaChunk {
	if a.spare != nil {
		ch := a.spare
		a.spare = ch.next
		ch.next = nil
		a.spareCount--
		return ch
	}

	a.chunkCount++
	return &arenaChunk{}
}

func (a *Arena) Alloc() *JSValue {
	if a == nil {
		return new(JSValue)
	}

	if a.current == nil {
		a.current = a.nextChunk()
	}

	if a.pos >= arenaChunkSize {
		nc := a.nextChunk()
		nc.next = a.current
		a.current = nc
		a.pos = 0
	}

	v := &a.current.slots[a.pos]
	a.pos++
	return v
}

func (a *Arena) PushScope() {
	if a == nil {
		return
	}
	if a.current == nil {
		a.current = a.nextChunk()
	}

	if n := len(a.marks); n < cap(a.marks) {
		a.marks = a.marks[:n+1]
		a.marks[n] = arenaMark{chunk: a.current, pos: a.pos}
		return
	}

	a.marks = append(a.marks, arenaMark{chunk: a.current, pos: a.pos})
}

func (a *Arena) PopScope() {
	if a == nil || len(a.marks) == 0 {
		return
	}

	mark := a.marks[len(a.marks)-1]
	a.marks = a.marks[:len(a.marks)-1]

	if a.current != nil && a.current != mark.chunk {
		head := a.current
		cur := a.current

		for cur != nil && cur.next != mark.chunk {
			cur = cur.next
		}

		if cur != nil {
			// Detach [head .. cur] from the live chain and move it to spare cache.
			detached := head
			cur.next = nil

			for detached != nil {
				next := detached.next
				detached.next = nil
				a.putSpare(detached)
				detached = next
			}
		}
	}

	a.current = mark.chunk
	a.pos = mark.pos
}

func (a *Arena) NewNumber(f float64) *JSValue {
	if v := maybeInternNumber(f); v != nil {
		return v
	}

	v := a.Alloc()
	v.typ = TypeNumber
	v.numVal = f
	v.prototype = NumberPrototype
	v.ext = nil
	v.boxedValue = nil
	v.funcVal = nil
	v.isArr = false
	v.isMethod = false
	v.frozen = false
	v.isArenaAllocated = true
	return v
}

func (a *Arena) NewBool(b bool) *JSValue { return NewBool(b) }

func (a *Arena) NewString(s string) *JSValue {
	if s == "" {
		return _emptyString
	}

	v := a.Alloc()
	v.typ = TypeString
	v.strVal = s
	v.prototype = StringPrototype
	v.ext = nil
	v.boxedValue = nil
	v.funcVal = nil
	v.isArr = false
	v.isMethod = false
	v.frozen = false
	v.isArenaAllocated = true
	return v
}

var arenaDisabled = os.Getenv("GUN_DISABLE_ARENA") == "1"
var arenaPool = sync.Pool{
	New: func() any { return NewArena() },
}

func GetArena() *Arena {
	if arenaDisabled {
		return nil
	}

	a := arenaPool.Get().(*Arena)
	a.reset()
	return a
}

func ReleaseArena(a *Arena) {
	if arenaDisabled || a == nil {
		return
	}

	a.reset()

	// If the arena grew too large during a spike, do not keep it in the pool.
	// Let it die so it doesn't permanently bloat pooled memory.
	if a.chunkCount > maxRetainedArenaChunks {
		return
	}

	arenaPool.Put(a)
}

func heapEscape(v *JSValue) *JSValue {
	if v == nil || !v.isArenaAllocated {
		return v
	}

	h := new(JSValue)
	*h = *v
	h.isArenaAllocated = false
	return h
}
