# TMDB Connectivity Test Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a settings-page button that performs a real, uncached server-side TMDB connectivity check and reports actionable failure reasons.

**Architecture:** A protected `GET /api/v1/tmdb/connectivity` endpoint calls TMDB `/configuration` through the existing server-side client without using the metadata cache. Stable client errors flow through the existing Problem response boundary, and a React Query mutation renders loading, success, and proxy-oriented failure feedback beside the configured status.

**Tech Stack:** Go 1.25, chi, testify, OpenAPI 3.1, React 19, TypeScript, TanStack React Query, Vitest, Testing Library, lucide-react.

---

### Task 1: Add an uncached TMDB client connectivity probe

**Files:**
- Modify: `internal/integrations/tmdb/client_test.go`
- Modify: `internal/integrations/tmdb/client.go`

**Step 1: Write the failing connectivity tests**

Add tests that call the desired `client.TestConnectivity(ctx)` method twice and assert that both calls reach `/configuration`, include the Bearer token, and succeed without reading from or writing to `tmdb_cache`. Add a table test asserting 401 and 403 return `ErrUnauthorized`, while other non-2xx responses remain `ErrUpstreamUnavailable`.

**Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/integrations/tmdb -run 'TestClientConnectivity' -count=1`

Expected: FAIL because `TestConnectivity` and `ErrUnauthorized` do not exist.

**Step 3: Implement the minimum client behavior**

Add:

```go
var ErrUnauthorized = errors.New("tmdb credentials were rejected")

func (client *Client) TestConnectivity(ctx context.Context) error {
    var response map[string]any
    return client.fetch(ctx, "/configuration", nil, &response)
}
```

Extract the current network request portion of `get` into an uncached `fetch` helper. Keep cache handling in `get`, and map HTTP 401/403 to `ErrUnauthorized` before the generic non-2xx branch. Do not persist the configuration response.

**Step 4: Run the focused package tests and verify GREEN**

Run: `go test ./internal/integrations/tmdb -count=1`

Expected: PASS.

**Step 5: Commit the client change**

```bash
git add internal/integrations/tmdb/client.go internal/integrations/tmdb/client_test.go
git commit -m "feat: probe TMDB connectivity"
```

### Task 2: Expose the protected connectivity endpoint and contract

**Files:**
- Modify: `internal/httpapi/tmdb_handlers_test.go`
- Modify: `internal/httpapi/contract_test.go`
- Modify: `internal/httpapi/tmdb_handlers.go`
- Modify: `internal/httpapi/router.go`
- Modify: `api/openapi.yaml`
- Regenerate: `web/src/api/generated.ts`

**Step 1: Write failing handler and route tests**

Add handler tests asserting:

```json
{"connected":true}
```

for a successful `/configuration` response, `tmdb_unauthorized` for 401/403, and existing stable codes for timeout, rate limit, and unavailable cases. Assert token and upstream bodies never appear in the response. Add `GET /api/v1/tmdb/connectivity` to the protected route contract test.

**Step 2: Run the focused HTTP tests and verify RED**

Run: `go test ./internal/httpapi -run 'TestTMDBConnectivity|TestOpenAPIProtectedRoutes' -count=1`

Expected: FAIL because the route and handler are missing.

**Step 3: Add the handler and route**

Implement:

```go
func (handlers tmdbHandlers) connectivity(w http.ResponseWriter, r *http.Request) {
    if err := handlers.client.TestConnectivity(r.Context()); err != nil {
        writeTMDBError(w, r, err)
        return
    }
    writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
}
```

Register the protected GET route and map `ErrUnauthorized` to HTTP 502 with Problem code `tmdb_unauthorized` so the application does not turn an upstream credential failure into a local login challenge.

**Step 4: Update and regenerate the API contract**

Add the path with operation ID `testTMDBConnectivity` and a `TMDBConnectivity` schema requiring `connected: boolean`. Run:

```bash
cd web && npm run api:generate && npm run api:check
```

Expected: generated types change only for the new path, schema, and operation.

**Step 5: Run the backend and contract tests**

Run: `go test ./internal/httpapi ./internal/integrations/tmdb -count=1`

Expected: PASS.

**Step 6: Commit the endpoint change**

```bash
git add internal/httpapi/tmdb_handlers.go internal/httpapi/tmdb_handlers_test.go internal/httpapi/router.go internal/httpapi/contract_test.go api/openapi.yaml web/src/api/generated.ts
git commit -m "feat: expose TMDB connectivity check"
```

### Task 3: Add the settings-page test button and feedback

**Files:**
- Modify: `web/src/features/settings/TmdbStatus.test.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/features/settings/TmdbStatus.tsx`
- Modify: `web/src/styles/global.css`

**Step 1: Write failing component tests**

Wrap the component with a test `QueryClientProvider`, mock the API method, and verify:

- configured state exposes a `测试连通` button;
- unconfigured state does not expose it;
- clicking shows disabled `正在测试` while pending;
- success displays `TMDB 连通正常` with status semantics;
- `tmdb_unauthorized`, `tmdb_timeout`, `tmdb_rate_limited`, and `tmdb_unavailable` map to distinct Chinese alerts, with the unavailable message explicitly mentioning server proxy or network settings.

**Step 2: Run the component test and verify RED**

Run: `cd web && npm test -- --run src/features/settings/TmdbStatus.test.tsx`

Expected: FAIL because the button and API function do not exist.

**Step 3: Add the API function and component mutation**

Add `testTMDBConnectivity()` to `web/src/api/client.ts`, calling `/api/v1/tmdb/connectivity`. In `TmdbStatus`, use `useMutation`, render a `RefreshCw` or `LoaderCircle` icon button labeled with text, and map stable `APIError.code` values to the approved copy. Clear the previous result at the start of a new test.

**Step 4: Add responsive styles**

Group the status label and button in an `.integration-status-actions` flex container. Keep the existing label appearance, use the repository's standard button styling, and allow wrapping without overlap on narrow screens. Style success and error feedback using existing color tokens and `role="status"` / `role="alert"` semantics.

**Step 5: Run frontend tests and static checks**

Run:

```bash
cd web
npm test -- --run src/features/settings/TmdbStatus.test.tsx src/app/App.test.tsx src/api/client.test.ts
npm run typecheck
npm run lint
npm run build
```

Expected: all commands PASS with no warnings.

**Step 6: Commit the UI change**

```bash
git add web/src/api/client.ts web/src/features/settings/TmdbStatus.tsx web/src/features/settings/TmdbStatus.test.tsx web/src/styles/global.css
git commit -m "feat: test TMDB connectivity from settings"
```

### Task 4: Document proxy troubleshooting and verify end to end

**Files:**
- Modify: `docs/integrations.md`
- Modify: `docker-compose.yml`
- Test: `scripts/docs-acceptance-test.sh`

**Step 1: Write a failing documentation acceptance assertion**

Require the integration documentation to mention `HTTPS_PROXY` and `NO_PROXY` for server-side TMDB requests.

**Step 2: Run the documentation test and verify RED**

Run: `bash scripts/docs-acceptance-test.sh`

Expected: FAIL because outbound proxy configuration is undocumented.

**Step 3: Add proxy configuration guidance**

Document that the connectivity test runs from the application container and show optional `HTTPS_PROXY` / `NO_PROXY` Compose entries. Add commented optional entries in `docker-compose.yml` without changing default runtime behavior or committing real proxy addresses.

**Step 4: Run full verification**

Run:

```bash
gofmt -w internal/integrations/tmdb/client.go internal/integrations/tmdb/client_test.go internal/httpapi/tmdb_handlers.go internal/httpapi/tmdb_handlers_test.go internal/httpapi/router.go internal/httpapi/contract_test.go
git diff --check
go test ./...
go vet ./...
bash scripts/docs-acceptance-test.sh
cd web && npm run api:check && npm test -- --run && npm run typecheck && npm run lint && npm run build
```

Expected: all commands PASS with no warnings.

**Step 5: Commit documentation and acceptance coverage**

```bash
git add docs/integrations.md docker-compose.yml scripts/docs-acceptance-test.sh
git commit -m "docs: explain TMDB proxy troubleshooting"
```
