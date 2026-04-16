// Package dynfunc provides runtime compilation of JavaScript function bodies
// into callable Go functions via HIR tree-walking interpretation.
//
// # Context Isolation
//
// The interpreter follows standard JavaScript semantics for scope isolation:
//
//   - new Function(paramNames..., body) creates a function in ISOLATED scope.
//     It can only access its own parameters and global variables (globalThis).
//     It CANNOT access closure variables from the surrounding scope.
//     This matches Bun, Node.js, and browser behavior.
//
//   - eval(code) in direct form evaluates in the CALLER's scope (can see locals).
//     Indirect eval (assigned to a variable first) runs in global scope only.
//     EvalHIR provides the "indirect eval" form — global scope only.
//
// # Synchronous Subset
//
// The HIR interpreter is a synchronous subset of JavaScript. Async/await and
// generators are handled by direct evaluation (no Promise/generator overhead).
// For full async semantics, use CompileFunction (Go plugin approach).
package dynfunc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"github.com/nnstd/gun/compiler/hir"
	"github.com/nnstd/gun/compiler/symbol"
	jsvalue "github.com/nnstd/gun/runtime/builtin"
	_ "github.com/nnstd/gun/runtime/builtin/console"
	_ "github.com/nnstd/gun/runtime/builtin/error"
	_ "github.com/nnstd/gun/runtime/builtin/json"
	_ "github.com/nnstd/gun/runtime/builtin/math"

	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func init() {
	jsvalue.CompileFunctionFn = CompileFunctionHIR
}

// --- Control flow signals (sent via panic/recover) ---

type signalReturn struct{ value *jsvalue.JSValue }
type signalBreak struct{}
type signalContinue struct{}

// Interpreter walks HIR nodes and evaluates them using JSValue.
type Interpreter struct {
	scopes   []map[symbol.ID]*jsvalue.JSValue
	globals  map[string]*jsvalue.JSValue
	thisVal  *jsvalue.JSValue
	superVal *jsvalue.JSValue // parent class for super calls
}

func newInterpreter() *Interpreter {
	interp := &Interpreter{
		globals: make(map[string]*jsvalue.JSValue),
	}
	interp.populateGlobals()
	return interp
}

// populateGlobals loads all registered JS global objects from the runtime/builtin
// global registry. Each runtime package self-registers via RegisterGlobal in its
// init(), so adding a new constructor/object anywhere automatically makes it
// available here.
//
// new Function() only sees these — no closure access.
func (interp *Interpreter) populateGlobals() {
	for name, val := range jsvalue.Globals() {
		interp.globals[name] = val
	}
}

func (interp *Interpreter) pushScope() {
	interp.scopes = append(interp.scopes, make(map[symbol.ID]*jsvalue.JSValue))
}

func (interp *Interpreter) popScope() {
	if len(interp.scopes) > 0 {
		interp.scopes = interp.scopes[:len(interp.scopes)-1]
	}
}

func (interp *Interpreter) set(sym *symbol.Symbol, val *jsvalue.JSValue) {
	if sym == nil {
		return
	}
	for i := len(interp.scopes) - 1; i >= 0; i-- {
		if _, ok := interp.scopes[i][sym.ID]; ok {
			interp.scopes[i][sym.ID] = val
			return
		}
	}
	if len(interp.scopes) > 0 {
		interp.scopes[len(interp.scopes)-1][sym.ID] = val
	} else {
		interp.globals[sym.OriginalName] = val
	}
}

func (interp *Interpreter) lookup(sym *symbol.Symbol) *jsvalue.JSValue {
	if sym == nil {
		return jsvalue.NewUndefined()
	}
	for i := len(interp.scopes) - 1; i >= 0; i-- {
		if v, ok := interp.scopes[i][sym.ID]; ok {
			return v
		}
	}
	if v, ok := interp.globals[sym.OriginalName]; ok {
		return v
	}
	return jsvalue.NewUndefined()
}

func (interp *Interpreter) lookupName(name string) *jsvalue.JSValue {
	if v, ok := interp.globals[name]; ok {
		return v
	}
	return jsvalue.NewUndefined()
}

// --- Expression evaluation ---

func (interp *Interpreter) evalExpr(e hir.Expr) *jsvalue.JSValue {
	if e == nil {
		return jsvalue.NewUndefined()
	}
	switch e := e.(type) {
	case *hir.Identifier:
		if e.Sym != nil {
			return interp.lookup(e.Sym)
		}
		return interp.lookupName(e.Name)

	case *hir.Literal:
		return interp.evalLiteral(e)

	case *hir.TemplateLiteral:
		return interp.evalTemplateLiteral(e)

	case *hir.TaggedTemplateLiteral:
		return interp.evalTaggedTemplate(e)

	case *hir.BinaryExpr:
		return interp.evalBinary(e)

	case *hir.UnaryExpr:
		return interp.evalUnary(e)

	case *hir.UpdateExpr:
		return interp.evalUpdate(e)

	case *hir.AssignExpr:
		return interp.evalAssign(e)

	case *hir.CallExpr:
		return interp.evalCall(e)

	case *hir.NewExpr:
		return interp.evalNew(e)

	case *hir.MemberExpr:
		obj := interp.evalExpr(e.Object)
		if e.Optional && (obj == nil || obj.Type() == jsvalue.TypeNull || obj.Type() == jsvalue.TypeUndefined) {
			return jsvalue.NewUndefined()
		}
		// Fast path: array.length
		if e.Property == "length" {
			return jsvalue.NewNumber(float64(obj.Len()))
		}
		return obj.Get(e.Property)

	case *hir.ComputedMemberExpr:
		obj := interp.evalExpr(e.Object)
		key := interp.evalExpr(e.Property)
		return obj.Get(key.String())

	case *hir.TernaryExpr:
		cond := interp.evalExpr(e.Cond)
		if cond.Bool() {
			return interp.evalExpr(e.Then)
		}
		return interp.evalExpr(e.Else)

	case *hir.ArrayLiteral:
		return interp.evalArrayLiteral(e)

	case *hir.ObjectLiteral:
		return interp.evalObjectLiteral(e)

	case *hir.ArrowFunc:
		return interp.evalFuncDecl(e.Params, e.Body, e.ExprBody)

	case *hir.FuncExpr:
		return interp.evalFuncDecl(e.Params, e.Body, nil)

	case *hir.ClassExpr:
		return interp.evalClassExpr(e)

	case *hir.SequenceExpr:
		var result *jsvalue.JSValue
		for _, ex := range e.Exprs {
			result = interp.evalExpr(ex)
		}
		return result

	case *hir.ParenExpr:
		return interp.evalExpr(e.Expr)

	case *hir.TypeAssertExpr:
		return interp.evalExpr(e.Expr)

	case *hir.NonNullExpr:
		return interp.evalExpr(e.Expr)

	case *hir.ThisExpr:
		if interp.thisVal != nil {
			return interp.thisVal
		}
		return jsvalue.NewUndefined()

	case *hir.SuperExpr:
		if interp.superVal != nil {
			return interp.superVal
		}
		return jsvalue.NewUndefined()

	case *hir.MetaPropertyExpr:
		// import.meta → empty object, new.target → undefined (not in constructor)
		if e.Meta == "import" && e.Property == "meta" {
			return jsvalue.NewObject()
		}
		return jsvalue.NewUndefined()

	case *hir.AwaitExpr:
		// Synchronous subset: evaluate directly, no Promise wrapping
		return interp.evalExpr(e.Value)

	case *hir.YieldExpr:
		// Synchronous subset: evaluate directly, no generator
		if e.Value != nil {
			return interp.evalExpr(e.Value)
		}
		return jsvalue.NewUndefined()

	case *hir.PrivateIdentifierExpr:
		// Private fields (#prop) — use thisVal + brand check
		if interp.thisVal != nil {
			return interp.thisVal.Get("__private_" + e.Name)
		}
		return jsvalue.NewUndefined()

	case *hir.SpreadExpr:
		return interp.evalExpr(e.Value)

	default:
		return jsvalue.NewUndefined()
	}
}

func (interp *Interpreter) evalLiteral(l *hir.Literal) *jsvalue.JSValue {
	switch l.Kind {
	case hir.LitString:
		s := l.Value
		if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
			s = s[1 : len(s)-1]
		}
		return jsvalue.NewString(s)
	case hir.LitNumber:
		f, err := strconv.ParseFloat(l.Value, 64)
		if err != nil {
			f = 0
		}
		return jsvalue.NewNumber(f)
	case hir.LitBool:
		return jsvalue.NewBool(l.Value == "true")
	case hir.LitNull:
		return jsvalue.NewNull()
	case hir.LitUndefined:
		return jsvalue.NewUndefined()
	case hir.LitBigInt:
		return jsvalue.NewString(l.Value + "n")
	case hir.LitRegex:
		// Return regex as string for now
		return jsvalue.NewString(l.Value)
	default:
		return jsvalue.NewUndefined()
	}
}

func (interp *Interpreter) evalTemplateLiteral(t *hir.TemplateLiteral) *jsvalue.JSValue {
	var sb strings.Builder
	for _, p := range t.Parts {
		v := interp.evalExpr(p)
		sb.WriteString(v.String())
	}
	return jsvalue.NewString(sb.String())
}

func (interp *Interpreter) evalTaggedTemplate(t *hir.TaggedTemplateLiteral) *jsvalue.JSValue {
	tagFn := interp.evalExpr(t.Tag)

	// Build template strings array (cooked values)
	var cooked []*jsvalue.JSValue
	var raw []*jsvalue.JSValue
	for _, p := range t.Template.Parts {
		if lit, ok := p.(*hir.Literal); ok && lit.Kind == hir.LitString {
			s := lit.Value
			cooked = append(cooked, jsvalue.NewString(s))
			raw = append(raw, jsvalue.NewString(s))
		} else {
			v := interp.evalExpr(p)
			cooked = append(cooked, v)
		}
	}

	// Create strings object with raw property
	stringsObj := jsvalue.NewArray(cooked...)
	rawObj := jsvalue.NewArray(raw...)
	stringsObj.Set("raw", rawObj)

	// Evaluate substitutions (non-string parts)
	var subs []*jsvalue.JSValue
	for _, p := range t.Template.Parts {
		if _, ok := p.(*hir.Literal); !ok {
			subs = append(subs, interp.evalExpr(p))
		}
	}

	// Call: tagFn(strings, ...subs)
	args := append([]*jsvalue.JSValue{stringsObj}, subs...)
	return tagFn.Call(args...)
}

func (interp *Interpreter) evalBinary(b *hir.BinaryExpr) *jsvalue.JSValue {
	// Short-circuit for logical ops
	switch b.Op {
	case hir.OpAnd:
		left := interp.evalExpr(b.Left)
		if !left.Bool() {
			return left
		}
		return interp.evalExpr(b.Right)
	case hir.OpOr:
		left := interp.evalExpr(b.Left)
		if left.Bool() {
			return left
		}
		return interp.evalExpr(b.Right)
	case hir.OpNullish:
		left := interp.evalExpr(b.Left)
		if left != nil && left.Type() != jsvalue.TypeNull && left.Type() != jsvalue.TypeUndefined {
			return left
		}
		return interp.evalExpr(b.Right)
	}

	left := interp.evalExpr(b.Left)
	right := interp.evalExpr(b.Right)

	switch b.Op {
	case hir.OpAdd:
		return jsvalue.Add(left, right)
	case hir.OpSub:
		return jsvalue.Sub(left, right)
	case hir.OpMul:
		return jsvalue.Mul(left, right)
	case hir.OpDiv:
		return jsvalue.Div(left, right)
	case hir.OpMod:
		return jsvalue.Mod(left, right)
	case hir.OpExp:
		return jsvalue.NewNumber(math.Pow(left.Number(), right.Number()))
	case hir.OpEq:
		return jsvalue.NewBool(jsvalue.Eq(left, right).Bool())
	case hir.OpNEq:
		return jsvalue.NewBool(!jsvalue.Eq(left, right).Bool())
	case hir.OpEqLoose:
		return jsvalue.NewBool(jsvalue.Eq(left, right).Bool()) // simplified: use strict
	case hir.OpNEqLoose:
		return jsvalue.NewBool(!jsvalue.Eq(left, right).Bool())
	case hir.OpLt:
		return jsvalue.Lt(left, right)
	case hir.OpGt:
		return jsvalue.Gt(left, right)
	case hir.OpLtE:
		return jsvalue.LtE(left, right)
	case hir.OpGtE:
		return jsvalue.GtE(left, right)
	case hir.OpBitAnd:
		return jsvalue.BitAnd(left, right)
	case hir.OpBitOr:
		return jsvalue.BitOr(left, right)
	case hir.OpBitXor:
		return jsvalue.BitXor(left, right)
	case hir.OpShl:
		return jsvalue.Shl(left, right)
	case hir.OpShr:
		return jsvalue.Shr(left, right)
	case hir.OpUShr:
		return jsvalue.UShr(left, right)
	case hir.OpIn:
		return jsvalue.NewBool(right.HasOwnProperty(left.String()))
	case hir.OpInstanceof:
		return jsvalue.NewBool(false) // simplified
	default:
		return jsvalue.NewUndefined()
	}
}

func (interp *Interpreter) evalUnary(u *hir.UnaryExpr) *jsvalue.JSValue {
	operand := interp.evalExpr(u.Operand)
	switch u.Op {
	case hir.OpNot:
		return jsvalue.Not(operand)
	case hir.OpNeg:
		return jsvalue.Neg(operand)
	case hir.OpPos:
		return jsvalue.NewNumber(operand.Number())
	case hir.OpBitNot:
		return jsvalue.BitNot(operand)
	case hir.OpTypeof:
		return jsvalue.TypeOf(operand)
	case hir.OpVoid:
		return jsvalue.NewUndefined()
	case hir.OpDelete:
		return jsvalue.NewBool(true)
	default:
		return operand
	}
}

func (interp *Interpreter) evalUpdate(u *hir.UpdateExpr) *jsvalue.JSValue {
	var current *jsvalue.JSValue
	var sym *symbol.Symbol

	if id, ok := u.Operand.(*hir.Identifier); ok {
		sym = id.Sym
		current = interp.evalExpr(u.Operand)
	} else {
		current = interp.evalExpr(u.Operand)
	}

	var delta float64 = 1
	if u.Op == hir.OpDec {
		delta = -1
	}

	newVal := jsvalue.NewNumber(current.Number() + delta)
	if u.Prefix {
		if sym != nil {
			interp.set(sym, newVal)
		}
		return newVal
	}
	// Postfix: return old value
	oldVal := current
	if sym != nil {
		interp.set(sym, newVal)
	}
	return oldVal
}

func (interp *Interpreter) evalAssign(a *hir.AssignExpr) *jsvalue.JSValue {
	// Handle destructuring assignment: {a, b} = obj
	if a.LeftPattern != nil {
		right := interp.evalExpr(a.Right)
		interp.bindPattern(a.LeftPattern, right)
		return right
	}

	right := interp.evalExpr(a.Right)

	if a.Op == hir.OpAssign {
		interp.assignTo(a.Left, right)
		return right
	}

	// Compound assignment
	left := interp.evalExpr(a.Left)
	var result *jsvalue.JSValue
	switch a.Op {
	case hir.OpAddAssign:
		result = jsvalue.Add(left, right)
	case hir.OpSubAssign:
		result = jsvalue.Sub(left, right)
	case hir.OpMulAssign:
		result = jsvalue.Mul(left, right)
	case hir.OpDivAssign:
		result = jsvalue.Div(left, right)
	case hir.OpModAssign:
		result = jsvalue.Mod(left, right)
	case hir.OpNullishAssign:
		if left != nil && left.Type() != jsvalue.TypeNull && left.Type() != jsvalue.TypeUndefined {
			result = left
		} else {
			result = right
		}
	case hir.OpOrAssign:
		if left.Bool() {
			result = left
		} else {
			result = right
		}
	case hir.OpAndAssign:
		if !left.Bool() {
			result = left
		} else {
			result = right
		}
	default:
		result = right
	}
	interp.assignTo(a.Left, result)
	return result
}

func (interp *Interpreter) assignTo(target hir.Expr, val *jsvalue.JSValue) {
	switch t := target.(type) {
	case *hir.Identifier:
		if t.Sym != nil {
			interp.set(t.Sym, val)
		}
	case *hir.MemberExpr:
		obj := interp.evalExpr(t.Object)
		obj.Set(t.Property, val)
	case *hir.ComputedMemberExpr:
		obj := interp.evalExpr(t.Object)
		key := interp.evalExpr(t.Property)
		obj.Set(key.String(), val)
	}
}

// expandArgs evaluates call arguments, expanding SpreadExpr into flat arg list.
func (interp *Interpreter) expandArgs(args []hir.Expr) []*jsvalue.JSValue {
	var expanded []*jsvalue.JSValue
	for _, a := range args {
		if spread, ok := a.(*hir.SpreadExpr); ok {
			val := interp.evalExpr(spread.Value)
			n := val.Len()
			for i := 0; i < n; i++ {
				expanded = append(expanded, val.Get(fmt.Sprintf("%d", i)))
			}
		} else {
			expanded = append(expanded, interp.evalExpr(a))
		}
	}
	return expanded
}

func (interp *Interpreter) evalCall(c *hir.CallExpr) *jsvalue.JSValue {
	fn := interp.evalExpr(c.Func)
	args := interp.expandArgs(c.Args)
	return fn.Call(args...)
}

func (interp *Interpreter) evalNew(n *hir.NewExpr) *jsvalue.JSValue {
	callee := interp.evalExpr(n.Callee)
	args := interp.expandArgs(n.Args)

	// Call constructor — it returns a new object
	result := callee.Call(args...)
	if result != nil && result.Type() == jsvalue.TypeObject {
		return result
	}
	// Fallback: create object and set constructor
	obj := jsvalue.NewObject()
	obj.Set("constructor", callee)
	return obj
}

func (interp *Interpreter) evalArrayLiteral(a *hir.ArrayLiteral) *jsvalue.JSValue {
	var elems []*jsvalue.JSValue
	for _, el := range a.Elements {
		if el == nil {
			elems = append(elems, jsvalue.NewUndefined())
		} else if spread, ok := el.(*hir.SpreadExpr); ok {
			val := interp.evalExpr(spread.Value)
			n := val.Len()
			for i := 0; i < n; i++ {
				elems = append(elems, val.Get(fmt.Sprintf("%d", i)))
			}
		} else {
			elems = append(elems, interp.evalExpr(el))
		}
	}
	return jsvalue.NewArray(elems...)
}

func (interp *Interpreter) evalObjectLiteral(o *hir.ObjectLiteral) *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	for _, p := range o.Properties {
		if spread, ok := p.Value.(*hir.SpreadExpr); ok {
			// Object spread: merge all own properties
			src := interp.evalExpr(spread.Value)
			for _, k := range src.OwnKeys() {
				obj.Set(k, src.Get(k))
			}
			continue
		}
		key := p.KeyName
		if p.Computed {
			key = interp.evalExpr(p.Key).String()
		} else if key == "" {
			if id, ok := p.Key.(*hir.Identifier); ok {
				key = id.Name
			}
		}
		val := interp.evalExpr(p.Value)
		if p.Method {
			// Method shorthand: { foo() {} } → set key to function
			obj.Set(key, val)
		} else {
			obj.Set(key, val)
		}
	}
	return obj
}

func (interp *Interpreter) evalClassExpr(c *hir.ClassExpr) *jsvalue.JSValue {
	// Build prototype chain
	proto := jsvalue.NewObject()
	ctor := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		instance := jsvalue.NewObject()
		// Set prototype chain
		instance.Set("__proto__", proto)

		// Bind properties
		for _, prop := range c.Properties {
			if prop.Value != nil {
				instance.Set(prop.Name, interp.evalExpr(prop.Value))
			}
		}

		// Run constructor
		if c.Constructor != nil {
			savedThis := interp.thisVal
			savedSuper := interp.superVal
			interp.thisVal = instance
			if c.Parent != nil {
				interp.superVal = interp.evalExpr(c.Parent)
			}
			defer func() {
				interp.thisVal = savedThis
				interp.superVal = savedSuper
			}()

			// Bind constructor params
			interp.pushScope()
			defer interp.popScope()
			for i, p := range c.Constructor.Params {
				if p.Symbol == nil {
					continue
				}
				val := jsvalue.NewUndefined()
				if i < len(args) && args[i] != nil {
					val = args[i]
				}
				if p.Default != nil && val.Type() == jsvalue.TypeUndefined {
					val = interp.evalExpr(p.Default)
				}
				interp.set(p.Symbol, val)
			}

			if c.Constructor.Body != nil {
				result := interp.execBlock(c.Constructor.Body)
				if ret, ok := result.(signalReturn); ok {
					return ret.value
				}
			}
		}
		return instance
	})

	// Attach methods to prototype
	for _, m := range c.Methods {
		method := m
		fn := jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
			savedThis := interp.thisVal
			interp.thisVal = args[0] // first arg is `this` in method call
			defer func() { interp.thisVal = savedThis }()

			interp.pushScope()
			defer interp.popScope()
			for i, p := range method.Params {
				if p.Symbol == nil {
					continue
				}
				val := jsvalue.NewUndefined()
				if i+1 < len(args) && args[i+1] != nil {
					val = args[i+1]
				}
				interp.set(p.Symbol, val)
			}
			if method.Body != nil {
				result := interp.execBlock(method.Body)
				if ret, ok := result.(signalReturn); ok {
					return ret.value
				}
			}
			return jsvalue.NewUndefined()
		})

		if m.IsStatic {
			ctor.Set(m.Name, fn)
		} else {
			proto.Set(m.Name, fn)
		}
	}

	ctor.Set("prototype", proto)
	return ctor
}

func (interp *Interpreter) evalFuncDecl(params []*hir.Param, body *hir.BlockStmt, exprBody hir.Expr) *jsvalue.JSValue {
	// Capture current scope for closure
	capturedScopes := make([]map[symbol.ID]*jsvalue.JSValue, len(interp.scopes))
	for i, s := range interp.scopes {
		cp := make(map[symbol.ID]*jsvalue.JSValue, len(s))
		for k, v := range s {
			cp[k] = v
		}
		capturedScopes[i] = cp
	}
	capturedGlobals := make(map[string]*jsvalue.JSValue, len(interp.globals))
	for k, v := range interp.globals {
		capturedGlobals[k] = v
	}
	thisVal := interp.thisVal
	superVal := interp.superVal

	return jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		// Per-call child interpreter: avoids shared mutable state.
		// Safe for concurrent calls (no mutex) and nested calls (no deadlock).
		child := &Interpreter{
			globals:  capturedGlobals,
			thisVal:  thisVal,
			superVal: superVal,
		}
		child.scopes = make([]map[symbol.ID]*jsvalue.JSValue, len(capturedScopes))
		for i, s := range capturedScopes {
			cs := make(map[symbol.ID]*jsvalue.JSValue, len(s))
			for k, v := range s {
				cs[k] = v
			}
			child.scopes[i] = cs
		}

		child.pushScope()

		// Bind parameters (including pattern parameters)
		for i, p := range params {
			if p.Pattern != nil {
				val := jsvalue.NewUndefined()
				if i < len(args) && args[i] != nil {
					val = args[i]
				}
				if p.Default != nil && val.Type() == jsvalue.TypeUndefined {
					val = child.evalExpr(p.Default)
				}
				child.bindPattern(p.Pattern, val)
				continue
			}
			if p.Symbol == nil {
				continue
			}
			if p.Rest {
				restElems := make([]*jsvalue.JSValue, 0)
				for j := i; j < len(args); j++ {
					restElems = append(restElems, args[j])
				}
				child.set(p.Symbol, jsvalue.NewArray(restElems...))
			} else {
				val := jsvalue.NewUndefined()
				if i < len(args) && args[i] != nil {
					val = args[i]
				}
				if p.Default != nil && val.Type() == jsvalue.TypeUndefined {
					val = child.evalExpr(p.Default)
				}
				child.set(p.Symbol, val)
			}
		}

		// Execute body
		if exprBody != nil {
			return child.evalExpr(exprBody)
		}
		if body != nil {
			result := child.execBlock(body)
			if ret, ok := result.(signalReturn); ok {
				return ret.value
			}
		}
		return jsvalue.NewUndefined()
	})
}

// --- Pattern matching ---

// bindPattern recursively binds a destructuring pattern from a source value.
func (interp *Interpreter) bindPattern(pat hir.Pattern, source *jsvalue.JSValue) {
	switch p := pat.(type) {
	case *hir.ObjectPattern:
		for _, prop := range p.Properties {
			var val *jsvalue.JSValue
			if source != nil {
				val = source.Get(prop.Key)
			} else {
				val = jsvalue.NewUndefined()
			}
			if prop.Default != nil && val.Type() == jsvalue.TypeUndefined {
				val = interp.evalExpr(prop.Default)
			}
			if prop.Value != nil {
				interp.set(prop.Value, val)
			} else if prop.Pattern != nil {
				interp.bindPattern(prop.Pattern, val)
			}
		}
		if p.Rest != nil {
			interp.set(p.Rest, source)
		}
	case *hir.ArrayPattern:
		for i, elem := range p.Elements {
			var val *jsvalue.JSValue
			if elem == nil {
				continue // hole: ,x,
			}
			if source != nil {
				val = source.Get(fmt.Sprintf("%d", i))
			} else {
				val = jsvalue.NewUndefined()
			}
			if elem.Default != nil && val.Type() == jsvalue.TypeUndefined {
				val = interp.evalExpr(elem.Default)
			}
			if elem.Symbol != nil {
				interp.set(elem.Symbol, val)
			} else if elem.Pattern != nil {
				interp.bindPattern(elem.Pattern, val)
			}
		}
		if p.Rest != nil {
			restStart := len(p.Elements)
			var restElems []*jsvalue.JSValue
			n := source.Len()
			for i := restStart; i < n; i++ {
				restElems = append(restElems, source.Get(fmt.Sprintf("%d", i)))
			}
			interp.set(p.Rest, jsvalue.NewArray(restElems...))
		}
	}
}

// --- Statement execution ---

type execResult interface{}

func (interp *Interpreter) execStmt(s hir.Stmt) execResult {
	if s == nil {
		return nil
	}
	switch s := s.(type) {
	case *hir.ExprStmt:
		interp.evalExpr(s.Expr)
	case *hir.VarDecl:
		interp.execVarDecl(s)
	case *hir.ReturnStmt:
		val := jsvalue.NewUndefined()
		if s.Value != nil {
			val = interp.evalExpr(s.Value)
		}
		return signalReturn{value: val}
	case *hir.IfStmt:
		cond := interp.evalExpr(s.Cond)
		if cond.Bool() {
			return interp.execBlock(s.Then)
		} else if s.Else != nil {
			return interp.execStmt(s.Else)
		}
	case *hir.BlockStmt:
		return interp.execBlock(s)
	case *hir.ForStmt:
		return interp.execFor(s)
	case *hir.WhileStmt:
		return interp.execWhile(s)
	case *hir.DoWhileStmt:
		return interp.execDoWhile(s)
	case *hir.ForOfStmt:
		return interp.execForOf(s)
	case *hir.ForInStmt:
		return interp.execForIn(s)
	case *hir.BreakStmt:
		return signalBreak{}
	case *hir.ContinueStmt:
		return signalContinue{}
	case *hir.ThrowStmt:
		val := interp.evalExpr(s.Value)
		panic(val)
	case *hir.TryCatchStmt:
		return interp.execTryCatch(s)
	case *hir.SwitchStmt:
		return interp.execSwitch(s)
	case *hir.EmptyStmt:
	case *hir.LabeledStmt:
		return interp.execStmt(s.Stmt)
	}
	return nil
}

func (interp *Interpreter) execBlock(b *hir.BlockStmt) execResult {
	if b == nil {
		return nil
	}
	interp.pushScope()
	defer interp.popScope()
	for _, s := range b.Stmts {
		result := interp.execStmt(s)
		if result != nil {
			return result
		}
	}
	return nil
}

func (interp *Interpreter) execVarDecl(d *hir.VarDecl) {
	for _, decl := range d.Declarators {
		var val *jsvalue.JSValue
		if decl.Init != nil {
			val = interp.evalExpr(decl.Init)
		} else {
			val = jsvalue.NewUndefined()
		}
		if decl.Symbol != nil {
			interp.set(decl.Symbol, val)
		} else if decl.Pattern != nil {
			interp.bindPattern(decl.Pattern, val)
		}
	}
}

func (interp *Interpreter) execFor(f *hir.ForStmt) execResult {
	interp.pushScope()
	defer interp.popScope()

	if f.Init != nil {
		interp.execStmt(f.Init)
	}

	for {
		if f.Cond != nil {
			cond := interp.evalExpr(f.Cond)
			if !cond.Bool() {
				break
			}
		}

		result := interp.execBlock(f.Body)
		switch result.(type) {
		case signalReturn:
			return result
		case signalBreak:
			return nil
		case signalContinue:
		}

		if f.Post != nil {
			interp.evalExpr(f.Post)
		}
	}
	return nil
}

func (interp *Interpreter) execWhile(w *hir.WhileStmt) execResult {
	for {
		cond := interp.evalExpr(w.Cond)
		if !cond.Bool() {
			break
		}
		result := interp.execBlock(w.Body)
		switch result.(type) {
		case signalReturn:
			return result
		case signalBreak:
			return nil
		case signalContinue:
		}
	}
	return nil
}

func (interp *Interpreter) execDoWhile(d *hir.DoWhileStmt) execResult {
	for {
		result := interp.execBlock(d.Body)
		switch result.(type) {
		case signalReturn:
			return result
		case signalBreak:
			return nil
		case signalContinue:
		}
		cond := interp.evalExpr(d.Cond)
		if !cond.Bool() {
			break
		}
	}
	return nil
}

func (interp *Interpreter) execForOf(f *hir.ForOfStmt) execResult {
	iterable := interp.evalExpr(f.Value)
	length := iterable.Len()

	for i := 0; i < length; i++ {
		elem := iterable.Get(fmt.Sprintf("%d", i))
		if f.Elem != nil {
			interp.set(f.Elem, elem)
		} else if f.Pattern != nil {
			interp.bindPattern(f.Pattern, elem)
		}

		result := interp.execBlock(f.Body)
		switch result.(type) {
		case signalReturn:
			return result
		case signalBreak:
			return nil
		case signalContinue:
		}
	}
	return nil
}

func (interp *Interpreter) execForIn(f *hir.ForInStmt) execResult {
	obj := interp.evalExpr(f.Value)
	keys := obj.OwnKeys()

	for _, key := range keys {
		interp.set(f.Key, jsvalue.NewString(key))

		result := interp.execBlock(f.Body)
		switch result.(type) {
		case signalReturn:
			return result
		case signalBreak:
			return nil
		case signalContinue:
		}
	}
	return nil
}

func (interp *Interpreter) execTryCatch(t *hir.TryCatchStmt) (result execResult) {
	defer func() {
		if r := recover(); r != nil {
			if t.Catch != nil {
				interp.pushScope()
				defer interp.popScope()
				if t.Catch.Param != nil {
					var errVal *jsvalue.JSValue
					switch v := r.(type) {
					case *jsvalue.JSValue:
						errVal = v
					case string:
						errVal = jsvalue.NewString(v)
					case error:
						errVal = jsvalue.NewString(v.Error())
					default:
						errVal = jsvalue.NewString(fmt.Sprintf("%v", v))
					}
					interp.set(t.Catch.Param, errVal)
				}
				result = interp.execBlock(t.Catch.Body)
			}
		}
	}()

	result = interp.execBlock(t.Try)
	if t.Finally != nil {
		finalResult := interp.execBlock(t.Finally)
		if finalResult != nil {
			return finalResult
		}
	}
	return result
}

func (interp *Interpreter) execSwitch(s *hir.SwitchStmt) execResult {
	tag := interp.evalExpr(s.Tag)

	for ci, c := range s.Cases {
		if c.Value != nil {
			caseVal := interp.evalExpr(c.Value)
			if !jsvalue.Eq(tag, caseVal).Bool() {
				continue
			}
		}
		// Matched (or default) — fall through remaining cases
		for _, stmt := range s.Cases[ci].Body {
			result := interp.execStmt(stmt)
			switch result.(type) {
			case signalReturn:
				return result
			case signalBreak:
				return nil
			}
		}
		// Fall through to next cases
		for cj := ci + 1; cj < len(s.Cases); cj++ {
			for _, stmt := range s.Cases[cj].Body {
				result := interp.execStmt(stmt)
				switch result.(type) {
				case signalReturn:
					return result
				case signalBreak:
					return nil
				}
			}
		}
		return nil
	}
	return nil
}

// --- Public API ---

// CompileFunctionHIR interprets a JS function body using the HIR interpreter.
// Context isolation: the compiled function can only access its own parameters
// and global variables — no closure access to the surrounding scope.
// This matches the behavior of new Function() in Bun, Node.js, and browsers.
func CompileFunctionHIR(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 {
		return jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
			return jsvalue.NewUndefined()
		})
	}

	body := args[len(args)-1].String()
	var paramNames []string
	for _, a := range args[:len(args)-1] {
		for _, p := range strings.Split(a.String(), ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				paramNames = append(paramNames, p)
			}
		}
	}

	// Check cache
	cacheKey := "hir:" + strings.Join(paramNames, ",") + "\x00" + body
	mu.Lock()
	if cache == nil {
		initCache()
	}
	if cached, ok := cache[cacheKey]; ok {
		mu.Unlock()
		return cached
	}
	mu.Unlock()

	// Parse JS → HIR
	params := strings.Join(paramNames, ", ")
	jsSource := fmt.Sprintf("function DynFunc(%s) { %s }", params, body)
	hirFn, err := parseToHIR(jsSource)
	if err != nil {
		return jsvalue.NewFunction(func(_args ...*jsvalue.JSValue) *jsvalue.JSValue {
			panic(jsvalue.NewString(fmt.Sprintf("CompileFunctionHIR error: %v", err)))
		})
	}

	mu.Lock()
	cache[cacheKey] = hirFn
	mu.Unlock()
	return hirFn
}

// EvalHIR evaluates a JS expression using the HIR interpreter.
// Context isolation: evaluates in global scope only (equivalent to indirect eval).
// Direct eval (which sees caller's local scope) is not supported — the HIR
// interpreter does not have access to the transpiled program's local variables.
func EvalHIR(code *jsvalue.JSValue) *jsvalue.JSValue {
	jsSource := fmt.Sprintf("function DynEval() { return (%s) }", code.String())

	fn, err := parseToHIR(jsSource)
	if err != nil {
		return jsvalue.NewString(fmt.Sprintf("EvalHIR error: %v", err))
	}
	return fn.Call()
}

// parseToHIR parses a JS function declaration and returns the function as a JSValue.
func parseToHIR(jsSource string) (*jsvalue.JSValue, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	lang := sitter.NewLanguage(typescript.LanguageTypescript())
	if err := parser.SetLanguage(lang); err != nil {
		return nil, fmt.Errorf("set language: %w", err)
	}

	source := []byte(jsSource)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("failed to parse JS: %s", jsSource)
	}
	defer tree.Close()

	hirMod := hir.BuildModuleWithPath(tree.RootNode(), source, "main", "")

	// Find the function declaration
	for _, d := range hirMod.Declarations {
		if fd, ok := d.(*hir.FuncDecl); ok {
			interp := newInterpreter()
			return interp.evalFuncDecl(fd.Params, fd.Body, nil), nil
		}
		if ed, ok := d.(*hir.ExportDecl); ok {
			if fd, ok := ed.Decl.(*hir.FuncDecl); ok {
				interp := newInterpreter()
				return interp.evalFuncDecl(fd.Params, fd.Body, nil), nil
			}
		}
	}

	return nil, fmt.Errorf("no function found in HIR")
}
