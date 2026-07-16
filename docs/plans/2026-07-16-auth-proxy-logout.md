# Proxy-Aware Authentication Logout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow login and logout behind an HTTPS-terminating reverse proxy, and make same-origin logout reliable when the tab CSRF state or server session is absent.

**Architecture:** Keep strict Origin host matching and derive only the effective request scheme from direct TLS or validated `Forwarded` / `X-Forwarded-Proto` headers. Route logout through same-origin protection without authentication or CSRF middleware, make the handler idempotently revoke when possible and always expire the cookie, then align the frontend and OpenAPI contract.

**Tech Stack:** Go 1.25, chi, net/http, SQLite, React 19, TypeScript, Vitest/MSW, OpenAPI 3.1, Playwright.

**Execution constraint:** The user explicitly requires direct work on `main`; execute sequentially in the current workspace without a worktree or subagents.

---

### Task 1: Honor the original HTTPS scheme behind a proxy

**Files:**
- Modify: `internal/httpapi/auth_handlers_test.go:59`
- Modify: `internal/httpapi/csrf.go:15`
- Modify: `docs/deployment.md:87`

**Step 1: Write the failing proxy login tests**

Add table-driven cases that initialize an administrator and submit correct credentials with an HTTPS Origin while the httptest request itself is HTTP:

```go
func TestLoginAcceptsHTTPSOriginForwardedByProxy(t *testing.T) {
	for name, headers := range map[string]map[string]string{
		"forwarded": {"Origin": "https://example.test", "Forwarded": "for=192.0.2.1;proto=https"},
		"x-forwarded-proto": {"Origin": "https://example.test", "X-Forwarded-Proto": "https"},
	} {
		t.Run(name, func(t *testing.T) {
			router, service := newAuthTestRouter(t, true)
			_, err := service.Initialize(context.Background(), "owner", "correct horse battery staple")
			require.NoError(t, err)
			response := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/login", map[string]string{
				"username": "owner", "password": "correct horse battery staple",
			}, headers)
			require.Equal(t, http.StatusOK, response.Code)
		})
	}
}
```

Add rejection cases for conflicting proxy headers, unsupported protocols, an incorrect Origin host, and a missing Origin. Direct HTTP and direct TLS behavior must remain covered.

**Step 2: Run the tests to verify RED**

Run: `go test ./internal/httpapi -run 'TestLogin(AcceptsHTTPSOriginForwardedByProxy|RejectsInvalidForwardedOrigin)' -count=1 -v`

Expected: the valid proxy cases fail with `expected 200, got 403` and `invalid_origin`; rejection cases pass.

**Step 3: Implement effective scheme parsing**

In `csrf.go`, keep `r.TLS` authoritative. When TLS is absent:

```go
func effectiveRequestScheme(r *http.Request) (string, bool) {
	if r.TLS != nil {
		return "https", true
	}
	forwarded, forwardedSet, forwardedOK := forwardedProto(r.Header.Values("Forwarded"))
	xForwarded, xForwardedSet, xForwardedOK := firstProxyProto(r.Header.Values("X-Forwarded-Proto"))
	if !forwardedOK || !xForwardedOK || (forwardedSet && xForwardedSet && forwarded != xForwarded) {
		return "", false
	}
	if forwardedSet {
		return forwarded, true
	}
	if xForwardedSet {
		return xForwarded, true
	}
	return "http", true
}
```

Parse the first proxy hop, compare parameter names case-insensitively, accept quoted RFC 7239 proto values, and allow only `http` / `https`. `RequireSameOrigin` must reject when parsing fails, compare the effective scheme to `origin.Scheme`, and continue comparing `origin.Host` to the unchanged `r.Host`.

**Step 4: Document the proxy headers**

Update the HTTPS reverse proxy checklist to require preserving Host/Origin and setting either `Forwarded: proto=https` or `X-Forwarded-Proto: https`. State that conflicting or invalid values are rejected.

**Step 5: Run GREEN verification**

Run: `gofmt -w internal/httpapi/csrf.go internal/httpapi/auth_handlers_test.go`

Run: `go test ./internal/httpapi -run 'Test(Login|CSRF|Origin)' -race -count=1 -v`

Expected: all selected authentication and origin tests pass under the race detector.

**Step 6: Commit**

```bash
git add internal/httpapi/csrf.go internal/httpapi/auth_handlers_test.go docs/deployment.md
git commit -m "fix: honor proxy scheme in origin checks"
```

### Task 2: Make same-origin logout idempotent

**Files:**
- Modify: `internal/httpapi/auth_handlers_test.go:89`
- Modify: `internal/httpapi/auth_handlers.go:105`
- Modify: `internal/httpapi/router.go:61`

**Step 1: Replace the old logout behavior test with failing desired behavior**

Keep missing and cross-site Origin cases at `403`. Assert that a same-origin request without CSRF returns `204`, revokes a valid session, and emits a cookie with an empty value, `Path=/`, and `MaxAge=-1`. Then assert an already revoked cookie and no cookie also return `204` with the same expired cookie.

```go
logout := performJSONRequest(router, http.MethodPost, "http://example.test/api/v1/auth/logout", nil, map[string]string{
	"Cookie": cookie.String(), "Origin": "http://example.test",
})
require.Equal(t, http.StatusNoContent, logout.Code)
require.Equal(t, -1, logout.Result().Cookies()[0].MaxAge)
_, err = service.Authenticate(context.Background(), cookie.Value)
require.ErrorIs(t, err, auth.ErrInvalidSession)
```

**Step 2: Run the logout test to verify RED**

Run: `go test ./internal/httpapi -run 'TestOriginProtectsIdempotentLogout' -count=1 -v`

Expected: the no-CSRF request returns `403`, and the no-cookie request returns `401` under the current route.

**Step 3: Implement the minimal handler and route**

Register logout next to login as `api.With(RequireSameOrigin).Post("/auth/logout", handlers.logout)` and remove it from the authenticated group.

Update the handler:

```go
func (handlers authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(SessionCookieName)
	if err == nil && cookie.Value != "" {
		if err := handlers.service.Revoke(r.Context(), cookie.Value); err != nil {
			writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
			return
		}
	}
	expired := handlers.sessionCookie("", time.Unix(1, 0).UTC())
	expired.MaxAge = -1
	http.SetCookie(w, expired)
	w.WriteHeader(http.StatusNoContent)
}
```

Treat `http.ErrNoCookie` as the expected no-session case. Do not suppress repository errors when an actual cookie was supplied.

**Step 4: Run GREEN verification**

Run: `gofmt -w internal/httpapi/auth_handlers.go internal/httpapi/auth_handlers_test.go internal/httpapi/router.go`

Run: `go test ./internal/auth ./internal/httpapi -run 'Test(Login|Session|Origin|Logout|CSRF)' -race -count=1 -v`

Expected: selected auth service and HTTP tests pass.

**Step 5: Commit**

```bash
git add internal/httpapi/auth_handlers.go internal/httpapi/auth_handlers_test.go internal/httpapi/router.go
git commit -m "fix: make same-origin logout idempotent"
```

### Task 3: Align the frontend and API contract

**Files:**
- Modify: `web/src/api/client.test.ts:1`
- Modify: `web/src/api/client.ts:317`
- Modify: `api/openapi.yaml:69`
- Modify: `internal/httpapi/contract_test.go:198`
- Regenerate: `web/src/api/generated.ts`
- Modify: `docs/security.md:11`

**Step 1: Write failing contract and frontend tests**

Change the OpenAPI contract test to assert logout does not document `CSRFToken`, while every route in `protectedWriteRoutes` still must document it. Add a client test that clears `sessionStorage`, calls `logoutUser`, and asserts the request has no `X-CSRF-Token` header.

**Step 2: Run RED verification**

Run: `go test ./internal/httpapi -run TestContractDefinesCursorETagAndProtectedWriteShapes -count=1 -v`

Expected: FAIL because logout still documents the CSRF parameter.

Run: `npm --prefix web test -- --run src/api/client.test.ts`

Expected: FAIL because `logoutUser` still sends an empty `X-CSRF-Token` header or is not imported by the test.

**Step 3: Update implementation and contract**

Make the frontend request independent of tab storage:

```ts
export function logoutUser() {
  return requestJSON<void>('/api/v1/auth/logout', { method: 'POST' })
}
```

Remove `CSRFToken` from the logout OpenAPI operation, describe `204` as an idempotent cookie/session clear, and regenerate types with `npm --prefix web run api:generate`. Update the security document to state that logout is same-origin and idempotent, while all other authenticated writes retain CSRF.

**Step 4: Run GREEN verification**

Run: `go test ./internal/httpapi -run 'TestContract' -count=1`

Run: `npm --prefix web test -- --run src/api/client.test.ts`

Run: `npm --prefix web run api:check`

Expected: all commands exit 0.

**Step 5: Commit**

```bash
git add api/openapi.yaml internal/httpapi/contract_test.go web/src/api/client.ts web/src/api/client.test.ts web/src/api/generated.ts docs/security.md
git commit -m "docs: align idempotent logout contract"
```

### Task 4: Verify the complete authentication workflow

**Files:**
- Modify: `task_plan.md`
- Modify: `findings.md`
- Modify: `progress.md`

**Step 1: Run backend and frontend gates**

Run: `go test ./... -race -count=1`

Run: `go vet ./...`

Run: `npm --prefix web test -- --run`

Run: `npm --prefix web run lint`

Run: `npm --prefix web run typecheck`

Run: `npm --prefix web run build`

Run: `npm --prefix web run api:check`

Run: `git diff --check`

Expected: every command exits 0 with no failures or warnings treated as errors.

**Step 2: Run isolated browser authentication verification**

Start the repository's isolated E2E environment. In a fresh browser context:

1. Initialize or use the synthetic administrator.
2. Log in through the page.
3. Clear only `sessionStorage` while keeping the HttpOnly cookie.
4. Reload and confirm `/auth/me` restores the authenticated page.
5. Click 退出登录 and confirm the login page appears without an error.
6. Log in again and confirm the application shell appears.
7. Confirm console warning/error count is zero and the authentication requests return `200`, `204`, and `200` in order.

Also send a local routed request with `Origin: https://...` plus a proxy protocol header and verify it reaches credential validation instead of returning `invalid_origin`.

**Step 3: Update tracking files**

Record RED/GREEN evidence, full command counts, browser request statuses, and any errors. Mark all five task phases complete only after fresh evidence exists.

**Step 4: Commit the verification record**

```bash
git add task_plan.md findings.md progress.md docs/plans/2026-07-16-auth-proxy-logout.md
git commit -m "docs: complete authentication reliability fix"
```
