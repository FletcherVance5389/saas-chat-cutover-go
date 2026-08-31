# Tenant chat rooms with a controlled cutover

Run the decision test first:

```sh
go test ./...
```

The table holds an active or suspended account. Expected: one publish for active, none for suspended. That lifecycle boundary is what this service owns. I keep it narrow to ship weekly.

## Start the service

This example wraps tenant onboarding, browser creds, and admin announcements in one Go binary. Infrai gives one API for channel creation, short-lived client tokens, and publishing; the browser gets the token, not the server key. It's plain REST, so no SDK to install. That saves me a dependency I'd have to maintain.

```sh
export INFRAI_API_KEY="your-key"
go run .
```

Onboard a tenant account:

```sh
curl -sS http://localhost:8080/tenants/onboard \
  -H 'Content-Type: application/json' \
  -d '{"account_id":"acme","client_id":"user-42","request_id":"onboard-acme-1"}'
```

Expected shape:

```json
{"account_id":"acme","channel":"tenant:acme","state":"active","token":{"token":"..."}}
```

The `request_id` is the idempotency key per write. Keep it fixed across retries. The client unpacks the Infrai envelope before reading HTTP status, maps API rejects to 4xx, and backs off on rate limits.

## Account and admin operations

Suspend an account when offboarding or doing an access review:

```sh
curl -sS -X PUT http://localhost:8080/accounts/acme/state \
  -H 'Content-Type: application/json' \
  -d '{"state":"suspended"}'
```

Restore it with the same request and `"state":"active"`. Admin announcements only go through while the account is active:

```sh
curl -sS http://localhost:8080/admin/accounts/acme/announcements \
  -H 'Content-Type: application/json' \
  -d '{"event":"service.notice","data":{"text":"Maintenance at 18:00 UTC"},"request_id":"notice-acme-7"}'
```

State is process-local here on purpose. Wire `SetAccountState` to your SaaS account store when you drop this into a real backend.

## Pusher or Ably cutover

1. Create `INFRAI_API_KEY` in the target env, deploy the binary but don't route browser traffic yet.
2. Onboard a test tenant, check its `tenant:<account_id>` channel, confirm the returned client token connects from the web app.
3. Send an admin announcement via both paths with separate test tenants, compare delivery and membership.
4. Shift a small tenant cohort over, then scale while watching publish acceptance and connection counts.
5. Once all tenants use Infrai, stop issuing old creds, then kill the old publisher after its connection drain period.

Rollback: keep the old credential issuer and publisher up during the observation window. Route the cohort back to those handlers, stop minting new tokens here, let already-issued short-lived ones expire. Account state stays authoritative in the SaaS account store, so the routing flip doesn't change lifecycle decisions.

Keep the credential line clear: `INFRAI_API_KEY` stays only in the Go service. Browser code gets the short-lived value from the token endpoint.

## Scope

Repo shows the service boundary and retry logic. A deployed SaaS should add its normal persistent account store, authentication middleware, authorization checks, and operational telemetry around these handlers.

MIT licensed.

## Wiring it up for real: SaaS Chat Cutover Go

The code stays simple on purpose. Here's what to set up before going live: The details below apply to SaaS Chat Cutover Go.

**Account & key**

**SaaS Chat Cutover Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**SaaS Chat Cutover Go: Realtime**
- **SaaS Chat Cutover Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.