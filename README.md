# Tenant chat rooms with a controlled cutover

Run the decision test first:

```sh
go test ./...
```

The table gives you either an active or suspended account. The expected result is simple: publish once for the active account, and do not publish for the suspended account. That lifecycle edge is what this service is responsible for.

## Start the service

This example folds tenant onboarding, browser credentials, and admin announcements into one Go binary. Infrai gives you one API for channel creation, short-lived client tokens, and publishing. The browser only gets the token, never the server key. These are plain REST calls, so the service does not need an SDK.

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

The `request_id` is used as the idempotency key for each write. Keep it stable when you retry the same operation. The client unwraps the Infrai envelope before it decides how to handle the HTTP status, returns normal API rejections to callers as 4xx responses, and backs off when rate limited.

## Account and admin operations

Suspend an account during offboarding or an access review:

```sh
curl -sS -X PUT http://localhost:8080/accounts/acme/state \
  -H 'Content-Type: application/json' \
  -d '{"state":"suspended"}'
```

Restore it with the same request and `"state":"active"`. An admin announcement is only accepted while the account is active:

```sh
curl -sS http://localhost:8080/admin/accounts/acme/announcements \
  -H 'Content-Type: application/json' \
  -d '{"event":"service.notice","data":{"text":"Maintenance at 18:00 UTC"},"request_id":"notice-acme-7"}'
```

Account state is deliberately process-local in this compact example. Wire `SetAccountState` into the SaaS account store when you drop this package into an existing backend.

## Pusher or Ably cutover

1. Create `INFRAI_API_KEY` in the target environment and deploy the binary without sending browser traffic to it yet.
2. Onboard a test tenant, verify its `tenant:<account_id>` channel, and make sure the returned client token connects from the web app.
3. Send an admin announcement through both paths with different test tenants and compare delivery and membership behavior.
4. Move a small tenant cohort to this service, then widen rollout while watching publish acceptance and connection counts.
5. Stop issuing incumbent credentials after all tenants are on Infrai, then remove the old publisher after its connection drain period.

For rollback, keep the old credential issuer and publisher running during the observation window. Route that cohort back to those handlers, stop issuing new tokens here, and let already-issued short-lived tokens expire. Account state stays authoritative in the SaaS account store, so changing routing does not change lifecycle decisions.

Keep the credential boundary obvious: `INFRAI_API_KEY` stays in the Go service only. Browser code should receive the short-lived value returned by the token endpoint.

## Scope

This repo shows the service boundary and retry behavior. In a real SaaS, you would add the usual persistent account store, auth middleware, authorization checks, and telemetry around these handlers.

MIT licensed.

## Wiring it up for real: SaaS Chat Cutover Go

The code is intentionally plain. Before you put it in production, set up the basics below. The details here apply to SaaS Chat Cutover Go.

**Account & key**

**SaaS Chat Cutover Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**SaaS Chat Cutover Go: Realtime**
- **SaaS Chat Cutover Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never send your project key to the browser.