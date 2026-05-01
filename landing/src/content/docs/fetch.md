---
title: Fetch
lead: Make HTTP requests using the standard Fetch API, fully compatible with browser and Node.js patterns.
sections:
  - Basic usage
  - JSON requests
  - Headers
  - POST and PUT
  - Streaming responses
  - AbortController
---

## Basic usage

```ts
const res = await fetch("https://api.example.com/data")
const text = await res.text()
```

## JSON requests

```ts
const res = await fetch("https://api.example.com/data")
const data = await res.json()
```

## Headers

Set request headers with the `Headers` object:

```ts
const headers = new Headers()
headers.set("Authorization", `Bearer ${token}`)
headers.set("Content-Type", "application/json")

const res = await fetch(url, { headers })
```

## POST and PUT

Send request bodies with any HTTP method:

```ts
const res = await fetch("https://api.example.com/items", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ name: "example", count: 42 }),
})

const created = await res.json()
```

## Streaming responses

Process large responses as streams:

```ts
const res = await fetch("https://api.example.com/large")
const reader = res.body?.getReader()
const decoder = new TextDecoder()

while (reader) {
  const { done, value } = await reader.read()
  if (done) break
  process.stdout.write(decoder.decode(value))
}
```

## AbortController

Cancel in-flight requests with `AbortController`:

```ts
const controller = new AbortController()

setTimeout(() => controller.abort(), 5000)

const res = await fetch("https://api.example.com/slow", {
  signal: controller.signal,
})
```

Aborted requests throw a `DOMException` with name `AbortError`.
