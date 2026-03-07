package symbol

import "fmt"

// Scope represents a lexical scope containing symbol definitions.
type Scope struct {
	Parent  *Scope
	symbols map[string]*Symbol // originalName → symbol
}

// Table is the central symbol table. It manages scopes and assigns unique IDs
// to every symbol definition. During code emission, EmitName generates
// collision-free Go identifiers using the symbol's unique ID when necessary.
type Table struct {
	nextID     ID
	scopeStack []*Scope
	current    *Scope
	allSymbols map[ID]*Symbol

	// emittedNames tracks Go names already emitted, for collision detection.
	// Maps generated Go name → symbol ID that owns it.
	emittedNames map[string]ID
}

// NewTable creates a new symbol table with a global scope.
func NewTable() *Table {
	global := &Scope{symbols: make(map[string]*Symbol)}
	return &Table{
		nextID:       1,
		scopeStack:   []*Scope{global},
		current:      global,
		allSymbols:   make(map[ID]*Symbol),
		emittedNames: make(map[string]ID),
	}
}

// PushScope creates and enters a new child scope.
func (t *Table) PushScope() *Scope {
	s := &Scope{
		Parent:  t.current,
		symbols: make(map[string]*Symbol),
	}
	t.scopeStack = append(t.scopeStack, s)
	t.current = s
	return s
}

// PopScope exits the current scope and returns to the parent.
// Panics if called on the global scope.
func (t *Table) PopScope() {
	if len(t.scopeStack) <= 1 {
		panic("symbol.Table: cannot pop global scope")
	}
	t.scopeStack = t.scopeStack[:len(t.scopeStack)-1]
	t.current = t.scopeStack[len(t.scopeStack)-1]
}

// Define creates a new symbol in the current scope with a unique ID.
// If a symbol with the same name already exists in the current scope, it is
// overwritten (redeclaration within the same scope).
func (t *Table) Define(name string, kind Kind) *Symbol {
	sym := &Symbol{
		ID:           t.nextID,
		OriginalName: name,
		Kind:         kind,
		IsJSValue:    true, // default in all-JSValue architecture
	}
	t.nextID++
	t.current.symbols[name] = sym
	t.allSymbols[sym.ID] = sym
	return sym
}

// Lookup searches for a symbol by name, starting from the current scope
// and walking up to parent scopes. Returns nil if not found.
func (t *Table) Lookup(name string) *Symbol {
	for s := t.current; s != nil; s = s.Parent {
		if sym, ok := s.symbols[name]; ok {
			return sym
		}
	}
	return nil
}

// LookupLocal searches for a symbol only in the current scope.
// Returns nil if not found in the current scope.
func (t *Table) LookupLocal(name string) *Symbol {
	return t.current.symbols[name]
}

// Get retrieves a symbol by its unique ID. Returns nil if not found.
func (t *Table) Get(id ID) *Symbol {
	return t.allSymbols[id]
}

// CurrentScope returns the current scope.
func (t *Table) CurrentScope() *Scope {
	return t.current
}

// IsGlobalScope returns true if the current scope is the global (outermost) scope.
func (t *Table) IsGlobalScope() bool {
	return len(t.scopeStack) == 1
}

// ScopeDepth returns the current scope nesting depth (0 = global).
func (t *Table) ScopeDepth() int {
	return len(t.scopeStack) - 1
}

// EmitName generates a collision-free Go identifier for the given symbol.
//
// Rules:
//   - Exported symbols get Capitalize(originalName)
//   - Non-exported symbols get Sanitize(originalName)
//   - If the resulting name collides with a previously emitted name owned by
//     a different symbol, a numeric suffix (_<id>) is appended
func (t *Table) EmitName(sym *Symbol) string {
	base := Sanitize(sym.OriginalName)
	if sym.Exported {
		base = Capitalize(base)
	}

	// Check if this name is already claimed
	if ownerID, taken := t.emittedNames[base]; taken && ownerID != sym.ID {
		// Collision — append symbol ID as suffix
		suffixed := fmt.Sprintf("%s_%d", base, sym.ID)
		t.emittedNames[suffixed] = sym.ID
		return suffixed
	}

	t.emittedNames[base] = sym.ID
	return base
}

// ReserveName marks a Go name as taken (e.g. for cross-file exports),
// associated with the given symbol. Returns the reserved name.
func (t *Table) ReserveName(name string, sym *Symbol) string {
	t.emittedNames[name] = sym.ID
	return name
}

// ForEach iterates over all symbols in the table.
func (t *Table) ForEach(fn func(sym *Symbol)) {
	for _, sym := range t.allSymbols {
		fn(sym)
	}
}
