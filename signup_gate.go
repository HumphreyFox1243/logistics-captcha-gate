package logisticsgate

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrInvalidSignup = errors.New("email, password, widget_record_id, captcha_token, and idempotency_key are required")

type SignupInput struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	Name           string `json:"name,omitempty"`
	WidgetRecordID string `json:"widget_record_id"`
	CaptchaToken   string `json:"captcha_token"`
	IdempotencyKey string `json:"idempotency_key"`
	IP             string `json:"-"`
}

type SignupResult struct {
	Accepted bool            `json:"accepted"`
	Account  json.RawMessage `json:"account"`
}

type SignupAPI interface {
	VerifyCaptcha(ctx context.Context, widgetRecordID, token, ip string) error
	CreateUser(ctx context.Context, input SignupInput) (json.RawMessage, error)
}

type SignupGate struct {
	api SignupAPI
}

func NewSignupGate(api SignupAPI) *SignupGate {
	return &SignupGate{api: api}
}

func (g *SignupGate) Register(ctx context.Context, input SignupInput) (SignupResult, error) {
	if input.Email == "" || input.Password == "" || input.WidgetRecordID == "" || input.CaptchaToken == "" || input.IdempotencyKey == "" {
		return SignupResult{}, ErrInvalidSignup
	}
	if err := g.api.VerifyCaptcha(ctx, input.WidgetRecordID, input.CaptchaToken, input.IP); err != nil {
		return SignupResult{}, err
	}
	account, err := g.api.CreateUser(ctx, input)
	if err != nil {
		return SignupResult{}, err
	}
	return SignupResult{Accepted: true, Account: account}, nil
}
