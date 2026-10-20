package logisticsgate

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type stubSignupAPI struct {
	verifyErr error
	createErr error
	calls     []string
}

func (s *stubSignupAPI) VerifyCaptcha(context.Context, string, string, string) error {
	s.calls = append(s.calls, "verify")
	return s.verifyErr
}

func (s *stubSignupAPI) CreateUser(context.Context, SignupInput) (json.RawMessage, error) {
	s.calls = append(s.calls, "create")
	return json.RawMessage(`{"id":"usr_test"}`), s.createErr
}

func TestSignupGateDecision(t *testing.T) {
	denied := &APIError{Code: "CAPTCHA_SCORE_TOO_LOW", Message: "captcha rejected", StatusCode: 422}
	tests := []struct {
		name      string
		input     SignupInput
		verifyErr error
		createErr error
		wantCalls []string
		accepted  bool
	}{
		{
			name:      "verified signup creates account",
			input:     SignupInput{Email: "ops@example.com", Password: "correct horse battery staple", WidgetRecordID: "widget-001", CaptchaToken: "verified-token", IdempotencyKey: "signup-001"},
			wantCalls: []string{"verify", "create"}, accepted: true,
		},
		{
			name:      "captcha rejection stops before account creation",
			input:     SignupInput{Email: "bot@example.com", Password: "correct horse battery staple", WidgetRecordID: "widget-001", CaptchaToken: "rejected-token", IdempotencyKey: "signup-002"},
			verifyErr: denied, wantCalls: []string{"verify"},
		},
		{
			name:      "missing replay key stops before remote calls",
			input:     SignupInput{Email: "ops@example.com", Password: "correct horse battery staple", CaptchaToken: "verified-token"},
			wantCalls: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &stubSignupAPI{verifyErr: tt.verifyErr, createErr: tt.createErr}
			result, err := NewSignupGate(api).Register(context.Background(), tt.input)
			if result.Accepted != tt.accepted {
				t.Fatalf("Accepted = %v, want %v", result.Accepted, tt.accepted)
			}
			if !reflect.DeepEqual(api.calls, tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", api.calls, tt.wantCalls)
			}
			if !tt.accepted && err == nil {
				t.Fatal("expected rejection error")
			}
		})
	}
}

func TestSignupGateSurfacesCreateFailure(t *testing.T) {
	want := errors.New("create failed")
	api := &stubSignupAPI{createErr: want}
	_, err := NewSignupGate(api).Register(context.Background(), SignupInput{
		Email: "ops@example.com", Password: "secret", WidgetRecordID: "widget-001", CaptchaToken: "verified-token", IdempotencyKey: "signup-003",
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
