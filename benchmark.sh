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
NODE_BIN="${NODE_BIN:-node}"
BENCH_DIR="/tmp/gun-bench"
K6_DURATION="${K6_DURATION:-10s}"
K6_RATE="${K6_RATE:-10000}"
K6_VUS="${K6_VUS:-100}"
GUN_OPT_LEVELS="${GUN_OPT_LEVELS:-0 1 2}"

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
  command -v $NODE_BIN >/dev/null 2>&1 || missing+=("node")
  command -v k6 >/dev/null 2>&1     || missing+=("k6")
  if [ ${#missing[@]} -gt 0 ]; then
    fail "Missing: ${missing[*]}"
  fi
  ok "Dependencies OK"
}

# ── Kill servers on our ports ───────────────────────────────

kill_servers() {
  for port in 3080 3081 3082 3083 3090 3091; do
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

measure_cold_start() {
  local port=$1
  shift
  local start_ns end_ns
  start_ns=$(python3 -c 'import time; print(time.monotonic_ns())')
  PORT=$port "$@" >/dev/null 2>&1 &
  local pid=$!
  local deadline=$((SECONDS + 10))
  while [ $SECONDS -lt $deadline ]; do
    if ! kill -0 "$pid" 2>/dev/null; then
      kill $pid 2>/dev/null || true
      echo "0"
      return 1
    fi
    if curl -sf "http://127.0.0.1:${port}/plaintext" >/dev/null 2>&1; then
      end_ns=$(python3 -c 'import time; print(time.monotonic_ns())')
      kill $pid 2>/dev/null || true
      wait $pid 2>/dev/null || true
      python3 -c "print(f'{(${end_ns}-${start_ns})/1_000_000:.1f}')"
      return 0
    fi
    sleep 0.01
  done
  kill $pid 2>/dev/null || true
  echo "0"
  return 1
}

wait_for_server() {
  local pid=$1
  local port=$2
  local timeout_s=${3:-5}
  local deadline=$((SECONDS + timeout_s))
  while [ $SECONDS -lt $deadline ]; do
    if ! kill -0 "$pid" 2>/dev/null; then
      return 1
    fi
    if curl -sf "http://127.0.0.1:${port}/plaintext" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

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

function fib(n: number): number {
  let a = 0;
  let b = 1;
  for (let i = 0; i < n; i++) {
    const next = a + b;
    a = b;
    b = next;
  }
  return a;
}

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

app.get("/fib/:n", (c) => {
  const raw = c.req.param("n");
  const n = Number(raw);
  if (!Number.isInteger(n) || n < 0 || n > 78) {
    return c.json({ error: "n must be an integer between 0 and 78" }, 400);
  }
  return c.json({ n, value: fib(n) });
});

Bun.serve({ port, fetch: app.fetch });
console.log(`Listening on ${port}`);
TYPESCRIPT
}

write_server_node_mjs() {
  cat > "$BENCH_DIR/node/server.mjs" << 'NODEJS'
import { Hono } from "hono";
import { serve } from "@hono/node-server";
import { z } from "zod";

const app = new Hono();
const port = Number(process.env.PORT || 3000);

const UserSchema = z.object({
  name: z.string().min(1),
  email: z.string().email(),
  age: z.number().min(0).max(150),
});

function fib(n) {
  let a = 0;
  let b = 1;
  for (let i = 0; i < n; i++) {
    const next = a + b;
    a = b;
    b = next;
  }
  return a;
}

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
app.get("/fib/:n", (c) => {
  const raw = c.req.param("n");
  const n = Number(raw);
  if (!Number.isInteger(n) || n < 0 || n > 78) {
    return c.json({ error: "n must be an integer between 0 and 78" }, 400);
  }
  return c.json({ n, value: fib(n) });
});

serve({ fetch: app.fetch, port });
console.log(`Listening on ${port}`);
NODEJS
}

# ── Bootstrap ───────────────────────────────────────────────

bootstrap() {
  log "Bootstrapping in $BENCH_DIR ..."

  rm -rf "$BENCH_DIR"
  mkdir -p "$BENCH_DIR/bun" "$BENCH_DIR/node" "$BENCH_DIR/logs"

  write_server_ts
  write_server_node_mjs

  # ── Gun: transpile + build for each opt level ──
  for level in $GUN_OPT_LEVELS; do
    log "Transpiling with Gun -O $level ..."
    mkdir -p "$BENCH_DIR/gun-o$level"
    $GUN_BIN transpile "$BENCH_DIR/server.ts" -o "$BENCH_DIR/gun-o$level" -O $level 2>&1 | tail -3
    (cd "$BENCH_DIR/gun-o$level" && go mod tidy 2>&1 && go build -ldflags="-s -w" -o bench-server . 2>&1)
    ok "Gun -O $level build done"
  done

  # ── Bun: install deps ──
  log "Installing Bun deps ..."
  cp "$BENCH_DIR/server.ts" "$BENCH_DIR/bun/server.ts"
  (cd "$BENCH_DIR/bun" && $BUN_BIN init -y 2>&1 | tail -1 && $BUN_BIN add hono zod 2>&1 | tail -3)
  ok "Bun setup done"

  # ── Node: install deps ──
  log "Installing Node deps ..."
  cat > "$BENCH_DIR/node/package.json" << 'PKG'
{
  "name": "gun-bench-node",
  "private": true,
  "type": "module",
  "dependencies": {
    "hono": "^4.12.15",
    "@hono/node-server": "^1.13.7",
    "zod": "^3.23.8"
  }
}
PKG
  (cd "$BENCH_DIR/node" && npm install --silent --no-audit --no-fund 2>&1 | tail -3)
  ok "Node setup done"
}

# ── Sample server resource usage during benchmark ───────────
# Polls ps for RSS (MB) and CPU% of the server PID.
# Writes one line per sample: "rss_mb cpu_pct" to outfile.
# Stops when kill -0 fails or stopfile is created.

sample_resources() {
  local pid=$1
  local outfile=$2
  local stopfile=$3

  > "$outfile"
  while [ ! -f "$stopfile" ] && kill -0 "$pid" 2>/dev/null; do
    local stats
    stats=$(ps -o rss= -o %cpu= -p "$pid" 2>/dev/null | awk '{printf "%.0f %.1f", $1/1024, $2}')
    echo "${stats:-0 0.0}" >> "$outfile"
    sleep 0.25
  done
}

# Parse resource sample file → peak_rss avg_cpu peak_cpu
parse_resources() {
  local infile=$1
  awk '
  {
    if ($1 > peak_rss) peak_rss = $1
    sum_cpu += $2; n++
    if ($2 > peak_cpu) peak_cpu = $2
  }
  END {
    printf "peak_rss=%.0f\navg_cpu=%.1f\npeak_cpu=%.1f\n", peak_rss, (n>0?sum_cpu/n:0), peak_cpu
  }' "$infile"
}

# ── Run k6 benchmark against a server ──────────────────────

run_k6() {
  local label=$1
  local port=$2
  local jsonfile=$3
  local logfile=$4
  local base="http://127.0.0.1:${port}"

  echo ""
  log "=== $label ==="
  echo ""

  : > "$logfile"
  env -u K6_DURATION -u K6_RATE -u K6_VUS k6 run --out json="$jsonfile" - >"$logfile" 2>&1 <<K6SCRIPT
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
  { method: 'GET',  url: '${base}/fib/32',                    name: 'GET /fib/:n' },
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
    "GET /fib/10:\"value\":55" \
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

# ── Extract metrics from parse_k6_json output into prefixed variables ──

extract_metrics() {
  local prefix=$1
  local output=$2
  while IFS='=' read -r k v; do
    eval "${prefix}_${k}=\"${v}\""
  done <<< "$output"
}

get_metric() {
  eval echo "\"\${${1}_${2}:-N/A}\""
}

# ── Main ────────────────────────────────────────────────────

main() {
  echo ""
  echo -e "${BOLD}========================================${RESET}"
  echo -e "${BOLD}  Hono + Zod Benchmark: Gun (O0/O1/O2) vs Bun${RESET}"
  echo -e "${BOLD}========================================${RESET}"
  echo ""

  check_deps
  bootstrap

  # ── Gun benchmarks for each opt level ──
  local port=3080
  for level in $GUN_OPT_LEVELS; do
    kill_servers
    local label="Gun -O${level}"
    log "Cold start: ${label} ..."
    local cs
    cs=$(measure_cold_start $port "$BENCH_DIR/gun-o${level}/bench-server")
    eval "gun_o${level}_cold_start=\"$cs\""
    kill_servers
    log "Starting ${label} server on :${port} ..."
    local server_log="$BENCH_DIR/logs/gun-o${level}.server.log"
    local k6_log="$BENCH_DIR/logs/gun-o${level}.k6.log"
    : > "$server_log"
    PORT=$port "$BENCH_DIR/gun-o${level}/bench-server" >"$server_log" 2>&1 &
    local pid=$!
    if ! wait_for_server "$pid" "$port" 5; then
      warn "${label} server did not become ready on :${port}, skipping"
      kill $pid 2>/dev/null || true
      port=$((port + 1))
      continue
    fi

    if verify_server "$label" $port; then
      local stopfile="$BENCH_DIR/gun-o${level}-res.stop"
      rm -f "$stopfile"
      sample_resources "$pid" "$BENCH_DIR/gun-o${level}-res.txt" "$stopfile" &
      local sampler_pid=$!

      run_k6 "$label" $port "$BENCH_DIR/gun-o${level}-results.json" "$k6_log"
      touch "$stopfile"
      wait $sampler_pid 2>/dev/null || true

      local m=$(parse_k6_json "$BENCH_DIR/gun-o${level}-results.json")
      extract_metrics "gun_o${level}" "$m"
      local r=$(parse_resources "$BENCH_DIR/gun-o${level}-res.txt")
      extract_metrics "gun_o${level}" "$r"
    else
      warn "${label} server verification failed, skipping (see $server_log)"
    fi

    kill $pid 2>/dev/null || true
    port=$((port + 1))
  done

  kill_servers

  # ── Bun benchmark ──
  local bun_port=3090
  log "Cold start: Bun ..."
  bun_cold_start=$(measure_cold_start $bun_port $BUN_BIN run "$BENCH_DIR/bun/server.ts")
  kill_servers
  log "Starting Bun server on :${bun_port} ..."
  local bun_server_log="$BENCH_DIR/logs/bun.server.log"
  local bun_k6_log="$BENCH_DIR/logs/bun.k6.log"
  : > "$bun_server_log"
  PORT=$bun_port $BUN_BIN run "$BENCH_DIR/bun/server.ts" >"$bun_server_log" 2>&1 &
  local bun_pid=$!
  if ! wait_for_server "$bun_pid" "$bun_port" 5; then
    warn "Bun server did not become ready on :${bun_port}, skipping (see $bun_server_log)"
    kill $bun_pid 2>/dev/null || true
    kill_servers
    return
  fi

  if verify_server "Bun" $bun_port; then
    local bun_stopfile="$BENCH_DIR/bun-res.stop"
    rm -f "$bun_stopfile"
    sample_resources "$bun_pid" "$BENCH_DIR/bun-res.txt" "$bun_stopfile" &
    local bun_sampler_pid=$!

    run_k6 "Bun" $bun_port "$BENCH_DIR/bun-results.json" "$bun_k6_log"
    touch "$bun_stopfile"
    wait $bun_sampler_pid 2>/dev/null || true

    local m=$(parse_k6_json "$BENCH_DIR/bun-results.json")
    extract_metrics "bun" "$m"
    local r=$(parse_resources "$BENCH_DIR/bun-res.txt")
    extract_metrics "bun" "$r"
  else
    warn "Bun server verification failed, skipping (see $bun_server_log)"
  fi

  kill $bun_pid 2>/dev/null || true
  kill_servers

  # ── Node benchmark ──
  local node_port=3091
  log "Cold start: Node ..."
  node_cold_start=$(measure_cold_start $node_port $NODE_BIN "$BENCH_DIR/node/server.mjs")
  kill_servers
  log "Starting Node server on :${node_port} ..."
  local node_server_log="$BENCH_DIR/logs/node.server.log"
  local node_k6_log="$BENCH_DIR/logs/node.k6.log"
  : > "$node_server_log"
  PORT=$node_port $NODE_BIN "$BENCH_DIR/node/server.mjs" >"$node_server_log" 2>&1 &
  local node_pid=$!
  if ! wait_for_server "$node_pid" "$node_port" 5; then
    warn "Node server did not become ready on :${node_port}, skipping (see $node_server_log)"
    kill $node_pid 2>/dev/null || true
    kill_servers
  elif verify_server "Node" $node_port; then
    local node_stopfile="$BENCH_DIR/node-res.stop"
    rm -f "$node_stopfile"
    sample_resources "$node_pid" "$BENCH_DIR/node-res.txt" "$node_stopfile" &
    local node_sampler_pid=$!

    run_k6 "Node" $node_port "$BENCH_DIR/node-results.json" "$node_k6_log"
    touch "$node_stopfile"
    wait $node_sampler_pid 2>/dev/null || true

    local m=$(parse_k6_json "$BENCH_DIR/node-results.json")
    extract_metrics "node" "$m"
    local r=$(parse_resources "$BENCH_DIR/node-res.txt")
    extract_metrics "node" "$r"
  else
    warn "Node server verification failed, skipping (see $node_server_log)"
  fi

  kill $node_pid 2>/dev/null || true
  kill_servers

  # ── Summary table ──
  echo ""
  echo -e "${BOLD}========================================${RESET}"
  echo -e "${BOLD}  Summary${RESET}"
  echo -e "${BOLD}========================================${RESET}"
  echo ""

  printf "${BOLD}%-18s %-14s %-14s %-14s %-14s %-14s${RESET}\n" "Metric" "Gun -O0" "Gun -O1" "Gun -O2" "Bun" "Node"

  for metric in rps err avg p50 p90 p95 peak_rss avg_cpu peak_cpu cold_start; do
    case "$metric" in
      rps) label="Throughput" ;;
      err) label="Error rate" ;;
      avg) label="Latency (avg)" ;;
      p50) label="Latency (p50)" ;;
      p90) label="Latency (p90)" ;;
      p95) label="Latency (p95)" ;;
      peak_rss) label="Peak RSS" ;;
      avg_cpu) label="CPU (avg)" ;;
      peak_cpu) label="CPU (peak)" ;;
      cold_start) label="Cold start" ;;
    esac
    local v0="$(get_metric gun_o0 $metric)"
    local v1="$(get_metric gun_o1 $metric)"
    local v2="$(get_metric gun_o2 $metric)"
    local vb="$(get_metric bun $metric)"
    local vn="$(get_metric node $metric)"
    [ "$metric" = "rps" ] && { v0="$v0 req/s"; v1="$v1 req/s"; v2="$v2 req/s"; vb="$vb req/s"; vn="$vn req/s"; }
    [ "$metric" = "peak_rss" ] && { v0="$v0 MB"; v1="$v1 MB"; v2="$v2 MB"; vb="$vb MB"; vn="$vn MB"; }
    [ "$metric" = "cold_start" ] && { v0="$v0 ms"; v1="$v1 ms"; v2="$v2 ms"; vb="$vb ms"; vn="$vn ms"; }
    [ "$metric" = "avg_cpu" ] || [ "$metric" = "peak_cpu" ] && { v0="$v0%"; v1="$v1%"; v2="$v2%"; vb="$vb%"; vn="$vn%"; }
    printf "%-18s %-14s %-14s %-14s %-14s %-14s\n" "$label" "$v0" "$v1" "$v2" "$vb" "$vn"
  done
  echo ""
}

main "$@"
