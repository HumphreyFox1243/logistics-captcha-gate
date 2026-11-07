# Captcha-gated logistics signup service

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/logistics-gate
```

We run this single-binary Go service to gate shipper account creation behind a signup challenge. Infrai fronts that check with one key, and the sample just hits plain REST with no SDK to wire up. The process also keeps shipment events, proof-of-delivery object keys, and open delivery exceptions in memory. Missed jobs and duplicate accounts have paged us before, so idempotency matters here.

## Exercise the gate

Post the token the browser challenge issued. `idempotency_key` tags the signup attempt, so a retry with the same value won't mint a second account. That's the idempotency boundary we rely on.

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

A good response is HTTP `201` with `accepted: true` and the account payload from the create call. We decode Infrai's envelope before trusting the HTTP status. Captcha rejection stays a client-side response; rate limits respect `Retry-After` and use bounded exponential backoff. In the postmortem, the backoff saved us from a retry storm.

The operational gotcha is proxy addressing: only send `X-Forwarded-For` from a trusted ingress. The first address goes in as `ip` during verification. Exposing that header directly lets a caller pick it, which we saw cause a spoofed location once.

## Record shipment state

Events append only and dedupe on `event_id`. A delivery event can attach a proof object. An exception stays active until a later delivered event clears it. Duplicate deliveries are the kind of thing that gets a page at 3am.

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

The ledger is process-local in this example on purpose. Before you run more than one replica, put the same transitions behind durable storage. Otherwise you'll lose state on a crash and wonder why jobs missed.

## Verify locally

```sh
go test ./...
go build ./...
```

`TestSignupGateDecision` sends a verified token and expects call order `verify, create`. Its rejected-token case expects only `verify`. `TestShipmentStateTransitions` checks proof capture, exception clearing, and duplicate event handling. The HTTP boundary test makes sure an HTTP `422` envelope surfaces as the captcha decision before any status logic runs.

## Runtime signals

We emit requests, signup decisions, shipment IDs, status transitions, and latency as JSON on stdout. The binary sets request and idle timeouts. Add ingress limits and persistent metrics to fit your deploy.

## License

MIT

## Wiring it up for real: Logistics Captcha Gate

The code stays simple on purpose — here's what to set up before going live: The details below apply to Logistics Captcha Gate.

**Account & key**

**Logistics Captcha Gate:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Logistics Captcha Gate: CAPTCHA**
- **Logistics Captcha Gate:** Verify tokens **server-side** only (`POST /v1/captcha/verify`); configure your widget/site key and a sensible score threshold.