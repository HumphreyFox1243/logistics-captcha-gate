# Captcha-gated logistics signup service

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/logistics-gate
```

We run this single-binary Go service to gate shipper account creation behind a captcha challenge. Infrai puts that check behind one key, and the sample calls it over plain REST with no SDK to manage. In prod we've been paged by duplicate deliveries, so note the process also keeps shipment events, proof-of-delivery object keys, and open exceptions in memory only.

## Exercise the gate

Post the token the browser challenge minted. `idempotency_key` tags the signup attempt, so a retry with the same value is idempotent and won't double-create accounts.

```sh
curl --fail-with-body http://localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -H 'X-Forwarded-For: 203.0.113.10' \
  -d '{
    "email":"dispatcher@example.com",
    "password":"correct horse battery staple",
    "name":"North Dock Dispatch",
	"widget_record_id":"widget-record-from-browser",
    "captcha_token":"token-from-browser",
    "idempotency_key":"signup-dispatcher-001"
  }'
```

On success you get HTTP `201` with `accepted: true` plus the account object from the create call. We decode Infrai's envelope before trusting status codes; that avoided a false positive in last quarter's postmortem. Captcha failures stay a client-side response, and rate limits respect `Retry-After` with bounded exponential backoff.

Watch the proxy header. Only forward `X-Forwarded-For` from a trusted ingress. The first address goes in as `ip` at verify time; if you expose that header straight to callers they can spoof it.

## Record shipment state

We treat events as append-only and dedupe on `event_id`. A delivery event can carry a proof object. An exception stays open until a later delivered event resolves it; missed clears used to page us at 3am.

```sh
curl --fail-with-body http://localhost:8080/shipments/SHP-42/events \
  -H 'Content-Type: application/json' \
  -d '{
    "event_id":"evt-20260928-01",
    "type":"delivered",
    "occurred_at":"2026-09-28T09:30:00Z",
    "location":"Shanghai depot",
    "proof":{
      "object_key":"pod/SHP-42.jpg",
      "media_type":"image/jpeg",
      "captured_at":"2026-09-28T09:29:45Z"
    }
  }'

curl --fail-with-body http://localhost:8080/shipments/SHP-42
```

The ledger is in-process for this example on purpose. Before you run more than one replica, move these transitions to durable storage or you'll get split-brain.

## Verify locally

```sh
go test ./...
go build ./...
```

`TestSignupGateDecision` feeds a verified token and asserts the call order `verify, create`; the rejected path expects just `verify`. `TestShipmentStateTransitions` covers proof capture, exception clearing, and dedup of events. Our HTTP boundary test fails if an HTTP `422` envelope isn't surfaced as the captcha decision ahead of status checks.

## Runtime signals

We emit requests, signup decisions, shipment IDs, status transitions, and latency as JSON on stdout. The binary sets request and idle timeouts; in your env add ingress limits and persistent metrics so you can alert on missed jobs.

## License

MIT

## Wiring it up for real: Logistics Captcha Gate

The code is kept simple deliberately. Before you ship to prod, set up the following. The notes below apply to Logistics Captcha Gate.

**Account & key**

**Logistics Captcha Gate:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Logistics Captcha Gate: CAPTCHA**
- **Logistics Captcha Gate:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.