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
  local jsonfile=$3
  local base="http://127.0.0.1:${port}"

  echo ""
  log "=== $label ==="
  echo ""

  k6 run --out json="$jsonfile" - <<K6SCRIPT
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

# ── Parse k6 JSON output for metrics ───────────────────────

parse_k6_json() {
  local jsonfile=$1
  python3 -c "
import json, sys

durations = []
total_reqs = 0
failed_reqs = 0
max_vus = 0
test_start = None
test_end = None

with open('$jsonfile') as f:
    for line in f:
        try:
            e = json.loads(line)
        except: continue
        t = e.get('type')
        m = e.get('metric','')
        data = e.get('data', {})

        if t == 'Point':
            if m == 'http_req_duration':
                durations.append(data['value'])  # k6 stores ms
            elif m == 'http_reqs':
                total_reqs += 1
            elif m == 'http_req_failed':
                if data.get('value', 0) > 0:
                    failed_reqs += 1
            elif m == 'vus_max':
                max_vus = max(max_vus, int(data['value']))
        elif t == 'VU':
            max_vus = max(max_vus, int(data['value']))

if not durations:
    print('ERROR:no_data')
    sys.exit(0)

durations.sort()
n = len(durations)
err_rate = (failed_reqs / total_reqs * 100) if total_reqs > 0 else 0

def pct(p):
    idx = int(n * p / 100)
    return durations[min(idx, n-1)]

def fmt_ms(ms):
    if ms < 1:
        return f'{ms*1000:.0f}µs'
    elif ms < 1000:
        return f'{ms:.2f}ms'
    else:
        return f'{ms/1000:.2f}s'

test_duration = ${K6_DURATION%s}
rps = total_reqs / test_duration if test_duration > 0 else 0

print(f'rps={rps:.0f}')
print(f'err={err_rate:.2f}%')
print(f'avg={fmt_ms(sum(durations)/n)}')
print(f'p50={fmt_ms(pct(50))}')
print(f'p90={fmt_ms(pct(90))}')
print(f'p95={fmt_ms(pct(95))}')
print(f'vus={max_vus}')
"
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

  local gun_metrics=""
  if verify_server "Gun" $GUN_PORT; then
    run_k6 "Gun (Go)" $GUN_PORT "$BENCH_DIR/gun-results.json"
    gun_metrics=$(parse_k6_json "$BENCH_DIR/gun-results.json")
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

  local bun_metrics=""
  if verify_server "Bun" $BUN_PORT; then
    run_k6 "Bun" $BUN_PORT "$BENCH_DIR/bun-results.json"
    bun_metrics=$(parse_k6_json "$BENCH_DIR/bun-results.json")
  else
    warn "Bun server verification failed, skipping benchmark"
  fi

  kill $bun_pid 2>/dev/null || true
  kill_servers

  # ── Extract metrics ──
  local gun_rps gun_err gun_avg gun_p50 gun_p90 gun_p95 gun_vus
  local bun_rps bun_err bun_avg bun_p50 bun_p90 bun_p95 bun_vus

  while IFS='=' read -r k v; do
    case "$k" in
      rps) gun_rps="$v" ;; err) gun_err="$v" ;; avg) gun_avg="$v" ;;
      p50) gun_p50="$v" ;; p90) gun_p90="$v" ;; p95) gun_p95="$v" ;; vus) gun_vus="$v" ;;
    esac
  done <<< "$gun_metrics"

  while IFS='=' read -r k v; do
    case "$k" in
      rps) bun_rps="$v" ;; err) bun_err="$v" ;; avg) bun_avg="$v" ;;
      p50) bun_p50="$v" ;; p90) bun_p90="$v" ;; p95) bun_p95="$v" ;; vus) bun_vus="$v" ;;
    esac
  done <<< "$bun_metrics"

  # ── Summary table ──
  echo ""
  echo -e "${BOLD}========================================${RESET}"
  echo -e "${BOLD}  Summary${RESET}"
  echo -e "${BOLD}========================================${RESET}"
  echo ""
  printf "${BOLD}%-20s %-15s %-15s${RESET}\n" "Metric" "Gun (Go)" "Bun"
  printf "%-20s %-15s %-15s\n" "Throughput" "${gun_rps:-N/A} req/s" "${bun_rps:-N/A} req/s"
  printf "%-20s %-15s %-15s\n" "Error rate" "${gun_err:-N/A}" "${bun_err:-N/A}"
  printf "%-20s %-15s %-15s\n" "Latency (avg)" "${gun_avg:-N/A}" "${bun_avg:-N/A}"
  printf "%-20s %-15s %-15s\n" "Latency (p50)" "${gun_p50:-N/A}" "${bun_p50:-N/A}"
  printf "%-20s %-15s %-15s\n" "Latency (p90)" "${gun_p90:-N/A}" "${bun_p90:-N/A}"
  printf "%-20s %-15s %-15s\n" "Latency (p95)" "${gun_p95:-N/A}" "${bun_p95:-N/A}"
  printf "%-20s %-15s %-15s\n" "VUs needed" "${gun_vus:-N/A}" "${bun_vus:-N/A}"
  echo ""
}

main "$@"
