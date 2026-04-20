package jsvalue

import "os"

const arenaChunkSize = 4096

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
	pos     int
	marks   []arenaMark
}

func NewArena() *Arena {
	return &Arena{current: &arenaChunk{}}
}

func (a *Arena) Destroy() {}

func (a *Arena) nextChunk() *arenaChunk {
	if a.spare != nil {
		ch := a.spare
		a.spare = ch.next
		ch.next = nil
		return ch
	}
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
			cur.next = a.spare
			a.spare = head
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

var globalArena = NewArena()
var arenaDisabled = os.Getenv("GUN_DISABLE_ARENA") == "1"

func GetArena() *Arena {
	if arenaDisabled {
		return nil
	}
	return globalArena
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
