---
title: Runtime fast paths for everyday JavaScript
slug: runtime-fast-paths
tag: Runtime
date: Apr 29, 2026
excerpt: A look at the small runtime shortcuts that make common string, array, object, and computed-property code cheaper after TypeScript becomes Go.
readTime: 7 min
color: '#7c8cff'
author: Nikita G.
authorRole: Runtime
---

Gun turns JavaScript and TypeScript into Go, but the generated program still has to behave like JavaScript. That means property lookup, string indexing, array methods, accessors, and computed keys all keep their dynamic semantics.

The expensive part is not one heroic operation. It is the thousand tiny operations that appear in ordinary application code:

```ts
const title = name.trim().toLowerCase()
const parts = path.split("/")
const padded = id.padStart(8, "0")

const rows = items
  .filter((item) => item.active)
  .map((item) => item.value)

const key = "content-type"
headers[key] = "application/json"
```

This pass focused on those everyday shapes. No new semantics, no new dependency, no compiler magic that only helps a benchmark. Just fewer allocations and less generic work on paths we already know are common.

## Fast strings when the value is already a string

Several string helpers used a generic conversion path before doing string work. That is correct, but it is too broad for the common case where the receiver is already a `JSValue` string.

```go
func stringFast(v *JSValue) string {
    if v != nil {
        if u := v.unboxed(); u != nil && u.typ == TypeString {
            return u.strVal
        }
    }
    return fmt.Sprint(v)
}
```

The fallback still preserves generic behavior. The fast path skips formatting for calls like `split`, `replace`, `startsWith`, `endsWith`, `repeat`, `substring`, and `slice` when the receiver is already a runtime string.

## ASCII does not need a rune allocation

JavaScript string indexing is character-oriented, so the runtime still has to handle non-ASCII input correctly. But much server-side JavaScript is dominated by ASCII: headers, paths, IDs, JSON keys, CLI flags, package names, and generated identifiers.

For those strings, converting to `[]rune` before every index or slice is unnecessary.

```ts
path.charAt(0)
name.slice(1, 8)
id.codePointAt(0)
```

The runtime now checks whether the string is ASCII and uses byte indexing when it can. Non-ASCII strings still fall back to the existing rune-aware behavior.

## Padding without repeated concatenation

The old `padStart` and `padEnd` implementation repeatedly converted the growing string to runes inside the loop. That made short code surprisingly expensive:

```ts
id.padStart(64, "0")
```

The new path computes the missing length once, repeats the pad string once, slices it to the exact character count, and joins the pieces.

```go
needed := targetLen - curLen
repeats := (needed + padLen - 1) / padLen
prefix := strings.Repeat(pad, repeats)
prefix = runeSliceFast(prefix, 0, needed)
return NewString(prefix + s)
```

It is still character-correct for non-ASCII padding, but avoids quadratic behavior for the common case.

## Arrays: hoist storage, preallocate results

Array methods are written against `JSValue`, so the runtime has to find the internal list behind each array. Doing that lookup for every element adds up.

This pass hoists the list once per method call:

```go
list := this.arrayListOrZero()
n := list.Len()
for i := range n {
    results[i] = fn.funcVal(list.Get(i), NewNumber(float64(i)), this)
}
```

That applies to the usual suspects: `map`, `filter`, `forEach`, `find`, `some`, `every`, `reduce`, `flat`, `flatMap`, `join`, `includes`, and related helpers.

We also preallocate result slices where the upper bound is already known:

```ts
items.filter((item) => item.enabled)
items.flat()
items.flatMap((item) => [item.id, item.name])
```

The runtime cannot know how many elements `filter` will keep, but it does know the result cannot exceed the input length. That is enough to avoid repeated slice growth.

## Building large arrays directly

Small arrays use inline storage, which is cheap. Large arrays used to be pushed one element at a time until the inline list spilled to heap storage.

For larger `NewArray(...)` calls, the runtime now initializes the backing list directly from a copied slice. This helps generated code that materializes arrays from `map`, `Object.keys`, string splits, or literal-like construction.

```ts
const values = [a, b, c, d, e, f, g, h, i]
```

Small arrays keep the compact inline path. Larger arrays skip the avoidable push-and-spill sequence.

## Object.values and Object.entries skip the second lookup

`Object.values(obj)` and `Object.entries(obj)` already enumerate own enumerable properties. The old implementation collected keys, then looked each key up again through `obj.Get(key)`.

That second lookup is semantically broader than needed. It can touch prototype logic, caches, and string-index behavior even though enumeration already found the own descriptor.

The new implementation copies enumerable descriptors under lock, releases the lock, then builds values or entries from those descriptors. Accessor properties still call their getter, just not while holding the object lock.

```ts
Object.values({ status: 200, ok: true })
Object.entries(headers)
```

Data properties now avoid the extra dynamic lookup entirely.

## Single-digit array indexes

Property access supports JavaScript's `arr["0"] === arr[0]` behavior. General index parsing is still needed for strings like `"123"`, but the hottest array indexes are often single digits.

```ts
args[0]
pair[1]
match[2]
```

The array property path now checks `"0"` through `"9"` before falling back to the general parser.

## Computed property keys avoid fmt.Sprint

Computed property access previously lowered many dynamic keys through `fmt.Sprint`:

```ts
obj[key]
rows[i] += word
obj[key]()
```

The compiler now emits `jsvalue.PropertyKey(key)` when the key expression is already a `JSValue`. The runtime helper handles common primitive keys directly:

```go
switch jsv.typ {
case TypeString:
    return jsv.strVal
case TypeNumber:
    return strconv.FormatFloat(jsv.numVal, 'g', -1, 64)
case TypeBoolean:
    return "true" // or "false"
case TypeSymbol:
    return fmt.Sprintf("@@sym%d:%s", jsv.symbolID, jsv.symbolDesc)
}
```

Generic formatting remains available as the fallback, but ordinary string and number keys avoid it.

## What improved

The benchmark suite added for this pass covers each targeted shape. On an Apple M1 Pro, representative results were:

```txt
String helpers:             ~1595 ns/op -> ~1206 ns/op
ASCII string indexing:      ~1173 ns/op ->  ~375 ns/op
String padding:             ~6198 ns/op ->  ~307 ns/op
Array loop methods:         ~5173 ns/op -> ~4533 ns/op
Array result preallocation: ~35127 ns/op -> ~26779 ns/op
Large NewArray:             ~1191 ns/op ->  ~333 ns/op
Object values/entries:      ~22416 ns/op -> ~13433 ns/op
Single-digit array get:     ~51.5 ns/op -> ~34.0 ns/op
PropertyKey runtime:        ~163 ns/op  ->  ~6.5 ns/op
```

The point is not that every program gets every number. The point is that common JavaScript idioms now pay less overhead in the runtime layer.

## The rule for fast paths

A fast path is only worth keeping if it stays boring:

- It must preserve JavaScript semantics.
- It must have a narrow fallback to the general path.
- It must be covered by a benchmark that resembles real generated code.
- It must make the common path cheaper without making the rare path fragile.

That is the shape of runtime optimization in Gun. The compiler can produce Go, but the runtime is where JavaScript's dynamic edges still show up. Fast paths make those edges cheaper without pretending they are not there.
