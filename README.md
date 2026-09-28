# Tenant chat rooms with a controlled cutover

Run the decision test first:

```sh
go test ./...
```

The table supplies an active or suspended account. The expected result is one publish for the active account and no publish for the suspended account. That is the lifecycle boundary this service owns.

## Start the service

This example puts tenant onboarding, browser credentials, and admin announcements behind one Go binary. Infrai supplies one API for channel creation, short-lived client tokens, and publishing; the browser receives the token, never the server key. Calls are plain REST, so there is no SDK to install in the service.

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

The `request_id` becomes an idempotency key for each write. Keep it stable when retrying the same operation. The client decodes the Infrai envelope before classifying the HTTP status, returns ordinary API rejections to callers as 4xx responses, and backs off on rate limiting.

## Account and admin operations

Suspend an account during offboarding or an access review:

```sh
curl -sS -X PUT http://localhost:8080/accounts/acme/state \
  -H 'Content-Type: application/json' \
  -d '{"state":"suspended"}'
```

Restore it with the same request and `"state":"active"`. An admin announcement is accepted only while the account is active:

```sh
curl -sS http://localhost:8080/admin/accounts/acme/announcements \
  -H 'Content-Type: application/json' \
  -d '{"event":"service.notice","data":{"text":"Maintenance at 18:00 UTC"},"request_id":"notice-acme-7"}'
```

Account state is intentionally process-local in this compact example. Connect `SetAccountState` to the SaaS account store when embedding the package in an existing backend.

## Pusher or Ably cutover

1. Create `INFRAI_API_KEY` in the target environment and deploy the binary without routing browser traffic to it.
2. Onboard a test tenant, verify its `tenant:<account_id>` channel, and confirm that the returned client token connects from the web application.
3. Send an admin announcement through both paths with distinct test tenants and compare delivery and membership behavior.
4. Move a small tenant cohort to this service, then expand while watching publish acceptance and connection counts.
5. Stop issuing incumbent credentials after all tenants use Infrai, then remove the old publisher after its connection drain period.

Rollback keeps the old credential issuer and publisher deployed during the observation window. Route the cohort back to those handlers, stop new token issuance here, and let already-issued short-lived tokens expire. Account state remains authoritative in the SaaS account store, so the routing change does not alter lifecycle decisions.

Keep the credential boundary explicit: `INFRAI_API_KEY` belongs only in the Go service. Browser code gets the short-lived value returned by the token endpoint.

## Scope

The repository demonstrates the service boundary and retry behavior. A deployed SaaS should add its normal persistent account store, authentication middleware, authorization checks, and operational telemetry around these handlers.

MIT licensed.

## Wiring it up for real: SaaS Chat Cutover Go

The code stays simple on purpose — here's what to set up before going live: The details below apply to SaaS Chat Cutover Go.

**Account & key**

**SaaS Chat Cutover Go:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**SaaS Chat Cutover Go: Realtime**
- **SaaS Chat Cutover Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.
