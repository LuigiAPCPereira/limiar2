---
name: testing-dashboard-security
description: Test limiar-collector dashboard security hardening — HTTP method enforcement, SSE subscriber limits, log redaction, and file permissions. Use when verifying dashboard security changes.
---

# Testing Dashboard Security Hardening

## Prerequisites

- Go installed (check `go version`)
- `CGO_ENABLED=1` for building (tursogo requires CGO)
- No external secrets needed for dashboard-only testing

## Build & Start Dashboard

```bash
export PATH=$PATH:/usr/local/go/bin
cd /home/ubuntu/repos/limiar-collector
CGO_ENABLED=1 go build -o /tmp/limiar-collector ./cmd/limiar-collector/

# Start dashboard in a temp directory (creates empty DB automatically)
mkdir -p /tmp/limiar-test && cd /tmp/limiar-test
export LIMIAR_APP_ID=12345 LIMIAR_API_HASH=fakehash123
/tmp/limiar-collector dashboard --port 9999 &
```

The dashboard creates `./limiar.db`, runs migrations, and binds to `127.0.0.1:9999`.

## Testing Approach

All tests are shell-based (curl + `go test`). No recording needed — changes are API-level, not visual UI changes.

### HTTP Method Enforcement

Test all registered endpoints with POST/PUT/DELETE (expect 405) and GET/HEAD (expect 200):

```bash
for endpoint in "/" "/healthz" "/api/channels" "/api/messages" "/api/message/1" "/api/processed" "/api/processed/stats"; do
  code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:9999${endpoint}")
  echo "POST $endpoint -> $code (expect 405)"
done
```

Use `--max-time 5` for HEAD requests to avoid hangs on streaming endpoints.

### SSE Subscriber Limit

The `/api/events` endpoint requires a broker (only initialized when the full collector pipeline runs with Telegram auth). In standalone dashboard mode, broker is nil and the route isn't registered (returns 404).

To test the subscriber limit, write a temporary Go test file inside `internal/dashboard/` that calls `NewBroker()` and `Subscribe()` 65 times, asserting the 65th returns `ErrMaxSubscribers`. Remove the test file after.

### Log Redaction

```bash
go test ./internal/logger/ -run TestProperty -v
```

This runs property-based tests (rapid) covering all sensitive keys including `secret`, `api_key`, `apikey`.

### File Permissions

```bash
grep -q 'OpenFile.*0600' tools/payload-analyzer/main.go && echo "PASS" || echo "FAIL"
grep -q 'os\.Create' tools/payload-analyzer/main.go && echo "FAIL: os.Create still present" || echo "PASS"
```

## Known Gotchas

- **`/api/events` not testable in standalone mode**: The SSE endpoint requires `broker != nil`, which only happens when the collector pipeline runs (needs Telegram credentials). Test via unit test on the Broker type instead.
- **HEAD requests may hang**: Some endpoints hold the connection open. Use `--max-time 5` with curl for HEAD requests.
- **CI `mattn/go-sqlite3` failure**: The CI script flags `mattn/go-sqlite3` as a forbidden import, but it's an indirect dependency of `tursogo` present on `main`. This might be fixed in the future — check if it persists before reporting as a new issue.
- **Go PATH**: Go might not be in PATH by default. Use `export PATH=$PATH:/usr/local/go/bin`.

## Devin Secrets Needed

None for dashboard-only testing. Full end-to-end SSE testing would require Telegram credentials (`LIMIAR_APP_ID`, `LIMIAR_API_HASH`, and an authenticated session).
