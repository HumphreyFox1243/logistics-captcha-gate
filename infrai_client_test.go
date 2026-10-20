package logisticsgate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientDecodesBusinessErrorBeforeHTTPStatus(t *testing.T) {
	client := NewInfraiClient("test-key", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/captcha/verify" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing bearer authorization")
		}
		var request captchaVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.WidgetRecordID != "widget-001" {
			t.Fatalf("widget_record_id = %q, want widget-001", request.WidgetRecordID)
		}
		return &http.Response{
			StatusCode: 422,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"CAPTCHA_SCORE_TOO_LOW","message":"captcha rejected"},"metadata":{}}`)),
		}, nil
	})})

	err := client.VerifyCaptcha(context.Background(), "widget-001", "rejected-token", "203.0.113.10")
	apiErr, ok := IsAPIError(err)
	if !ok || apiErr.Code != "CAPTCHA_SCORE_TOO_LOW" || apiErr.StatusCode != 422 {
		t.Fatalf("error = %#v", err)
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	attempts := 0
	client := NewInfraiClient("test-key", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		status := http.StatusOK
		body := `{"ok":true,"data":{"verified":true},"error":null,"metadata":{}}`
		header := make(http.Header)
		if attempts == 1 {
			status = http.StatusTooManyRequests
			body = `{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`
			header.Set("Retry-After", "0")
		}
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	client.sleep = func(context.Context, time.Duration) error { return nil }

	if err := client.VerifyCaptcha(context.Background(), "widget-001", "verified-token", "203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}
