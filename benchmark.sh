#!/bin/bash
set -uo pipefail

# ─────────────────────────────────────────────────────────────
# Hono + Zod Benchmark: Gun (Go) vs Bun
#
# Bootstraps everything from scratch in /tmp. Requires:
#   - Go, Bun, k6 in PATH
# ─────────────────────────────────────────────────────────────

GUN_DIR="$(cd "$(dirname "$0")" && pwd)"
GUN_BIN="${GUN_BIN:-go run $GUN_DIR}"
BUN_BIN="${BUN_BIN:-bun}"
GUN_PORT=3080
BUN_PORT=3081
BENCH_DIR="/tmp/gun-bench"
K6_DURATION="${K6_DURATION:-10s}"
K6_RATE="${K6_RATE:-10000}"
K6_VUS="${K6_VUS:-100}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
RESET='\033[0m'

log()   { echo -e "${BOLD}${CYAN}$1${RESET}"; }
ok()    { echo -e "${GREEN}${BOLD}$1${RESET}"; }
warn()  { echo -e "${YELLOW}${BOLD}$1${RESET}"; }
fail()  { echo -e "${RED}${BOLD}$1${RESET}"; exit 1; }

# ── Dependency checks ──────────────────────────────────────

check_deps() {
  local missing=()
  command -v go >/dev/null 2>&1      || missing+=("go")
  command -v $BUN_BIN >/dev/null 2>&1 || missing+=("bun")
  command -v k6 >/dev/null 2>&1     || missing+=("k6")
  if [ ${#missing[@]} -gt 0 ]; then
    fail "Missing: ${missing[*]}"
  fi
  ok "Dependencies OK"
}

# ── Kill servers on our ports ───────────────────────────────

kill_servers() {
  for port in $GUN_PORT $BUN_PORT; do
    local pid=$(lsof -ti :$port 2>/dev/null || true)
    if [ -n "$pid" ]; then
      kill $pid 2>/dev/null || true
    fi
  done
  sleep 0.3
}

cleanup() {
  kill_servers
}
trap cleanup EXIT

# ── Generate server source ──────────────────────────────────

write_server_ts() {
  cat > "$BENCH_DIR/server.ts" << 'TYPESCRIPT'
import { Hono } from "hono";
import { z } from "zod";

const app = new Hono();
const port = Number(process.env.PORT || 3000);

const UserSchema = z.object({
  name: z.string().min(1),
  email: z.string().email(),
  age: z.number().min(0).max(150),
});

app.get("/plaintext", (c) => c.text("Hello, World!"));

app.get("/json", (c) => c.json({ hello: "world", ts: Date.now() }));

app.get("/users/:id", (c) => {
  const id = c.req.param("id");
  return c.json({ id, name: "User " + id });
});

app.post("/users", async (c) => {
  const body = await c.req.json();
  const result = UserSchema.safeParse(body);
  if (!result.success) {
    return c.json({ error: "validation failed" }, 400);
  }
  return c.json({ created: true, ...result.data }, 201);
});

app.get("/items/:itemId/comments/:commentId", (c) => {
  return c.json({
    item: c.req.param("itemId"),
    comment: c.req.param("commentId"),
  });
});

Bun.serve({ port, fetch: app.fetch });
console.log(`Listening on ${port}`);
TYPESCRIPT
}

# ── Bootstrap ───────────────────────────────────────────────

bootstrap() {
  log "Bootstrapping in $BENCH_DIR ..."

  rm -rf "$BENCH_DIR"
  mkdir -p "$BENCH_DIR/gun-out" "$BENCH_DIR/bun"

  write_server_ts

  # ── Gun: transpile + build ──
  log "Transpiling with Gun ..."
  PORT=$GUN_PORT $GUN_BIN transpile "$BENCH_DIR/server.ts" -o "$BENCH_DIR/gun-out" 2>&1 | tail -3
  (cd "$BENCH_DIR/gun-out" && go mod tidy 2>&1 && go build -o bench-server . 2>&1)
  ok "Gun build done"

  # ── Bun: install deps ──
  log "Installing Bun deps ..."
  cp "$BENCH_DIR/server.ts" "$BENCH_DIR/bun/server.ts"
  (cd "$BENCH_DIR/bun" && $BUN_BIN init -y 2>&1 | tail -1 && $BUN_BIN add hono zod 2>&1 | tail -3)
  ok "Bun setup done"
}

# ── Run k6 benchmark against a server ──────────────────────

run_k6() {
  local label=$1
  local port=$2
  local base="http://127.0.0.1:${port}"

  echo ""
  log "=== $label ==="
  echo ""

  k6 run - <<K6SCRIPT
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: ${K6_RATE},
      timeUnit: '1s',
      duration: '${K6_DURATION}',
      preAllocatedVUs: ${K6_VUS},
      maxVUs: ${K6_VUS},
    },
  },
  thresholds: { http_req_failed: ['rate<1'] },
};

const routes = [
  { method: 'GET',  url: '${base}/plaintext',                name: 'GET /plaintext' },
  { method: 'GET',  url: '${base}/json',                      name: 'GET /json' },
  { method: 'GET',  url: '${base}/users/42',                  name: 'GET /users/:id' },
  { method: 'GET',  url: '${base}/items/7/comments/99',       name: 'GET /items/:id/comments/:id' },
  { method: 'POST', url: '${base}/users',                     name: 'POST /users',
    body: '{"name":"Alice","email":"alice@example.com","age":30}',
    headers: { 'Content-Type': 'application/json' } },
];

export default function () {
  const r = routes[Math.floor(Math.random() * routes.length)];
  const params = { headers: r.headers || {}, tags: { route: r.name } };
  let res;
  if (r.method === 'POST') {
    res = http.post(r.url, r.body, params);
  } else {
    res = http.get(r.url, params);
  }
  check(res, { 'status 2xx/3xx': (s) => s.status >= 200 && s.status < 400 });
}
K6SCRIPT
}

# ── Verify a server responds correctly ─────────────────────

verify_server() {
  local label=$1
  local port=$2
  local base="http://127.0.0.1:${port}"
  local all_ok=true

  echo "  Verifying routes ..."
  for check in \
    "GET /plaintext:Hello, World!" \
    "GET /json:hello" \
    "GET /users/42:User 42" \
    "GET /items/7/comments/99:item" \
    "POST /users:created"; do
    local route="${check%%:*}"
    local expect="${check#*:}"
    local method="${route%% *}"
    local path="${route#* }"
    local resp

    if [ "$method" = "POST" ]; then
      resp=$(curl -sf -X POST -H "Content-Type: application/json" \
        -d '{"name":"Alice","email":"alice@example.com","age":30}' \
        "${base}${path}" 2>&1) || { all_ok=false; echo "    FAIL $route (curl error)"; continue; }
    else
      resp=$(curl -sf "${base}${path}" 2>&1) || { all_ok=false; echo "    FAIL $route (curl error)"; continue; }
    fi

    if echo "$resp" | grep -q "$expect"; then
      echo "    OK   $route"
    else
      all_ok=false
      echo "    FAIL $route — expected '$expect', got: $(echo "$resp" | head -c 80)"
    fi
  done

  [ "$all_ok" = true ]
}

# ── Main ────────────────────────────────────────────────────

main() {
  echo ""
  echo -e "${BOLD}========================================${RESET}"
  echo -e "${BOLD}  Hono + Zod Benchmark: Gun vs Bun${RESET}"
  echo -e "${BOLD}========================================${RESET}"
  echo ""

  check_deps
  bootstrap

  # ── Gun benchmark ──
  kill_servers
  log "Starting Gun server on :$GUN_PORT ..."
  PORT=$GUN_PORT "$BENCH_DIR/gun-out/bench-server" &
  local gun_pid=$!
  sleep 1

  if verify_server "Gun" $GUN_PORT; then
    run_k6 "Gun (Go)" $GUN_PORT
  else
    warn "Gun server verification failed, skipping benchmark"
  fi

  kill $gun_pid 2>/dev/null || true
  kill_servers

  # ── Bun benchmark ──
  log "Starting Bun server on :$BUN_PORT ..."
  PORT=$BUN_PORT $BUN_BIN run "$BENCH_DIR/bun/server.ts" &
  local bun_pid=$!
  sleep 1

  if verify_server "Bun" $BUN_PORT; then
    run_k6 "Bun" $BUN_PORT
  else
    warn "Bun server verification failed, skipping benchmark"
  fi

  kill $bun_pid 2>/dev/null || true
  kill_servers

  echo ""
  echo -e "${BOLD}========================================${RESET}"
  echo -e "${BOLD}  Done!${RESET}"
  echo -e "${BOLD}========================================${RESET}"
}

main "$@"
