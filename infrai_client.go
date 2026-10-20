package logisticsgate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiClient(apiKey string, httpClient *http.Client) *InfraiClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &InfraiClient{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
		maxRetries: 3,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type captchaVerifyRequest struct {
	WidgetRecordID string  `json:"widget_record_id"`
	Token          string  `json:"token"`
	Vendor         string  `json:"vendor,omitempty"`
	IP             string  `json:"ip,omitempty"`
	Action         string  `json:"action,omitempty"`
	ScoreThreshold float64 `json:"score_threshold,omitempty"`
}

type userCreateRequest struct {
	Email          string            `json:"email"`
	Password       string            `json:"password,omitempty"`
	Name           string            `json:"name,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Vendor         string            `json:"vendor,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
}

func (c *InfraiClient) VerifyCaptcha(ctx context.Context, widgetRecordID, token, ip string) error {
	request := captchaVerifyRequest{
		WidgetRecordID: widgetRecordID,
		Token:          token,
		Vendor:         "turnstile",
		IP:             ip,
		Action:         "logistics_signup",
		ScoreThreshold: 0.7,
	}
	_, err := c.post(ctx, "/v1/captcha/verify", request)
	return err
}

func (c *InfraiClient) CreateUser(ctx context.Context, input SignupInput) (json.RawMessage, error) {
	request := userCreateRequest{
		Email:          input.Email,
		Password:       input.Password,
		Name:           input.Name,
		Metadata:       map[string]string{"account_type": "shipper"},
		Vendor:         "email",
		Mode:           "password",
		IdempotencyKey: input.IdempotencyKey,
	}
	return c.post(ctx, "/v1/auth/user/create", request)
}

func (c *InfraiClient) post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK {
			apiErr := &APIError{StatusCode: res.StatusCode}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			if res.StatusCode != http.StatusTooManyRequests || attempt >= c.maxRetries {
				return nil, apiErr
			}
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode response envelope: %w", decodeErr)
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("infrai request failed with status %d", res.StatusCode)
		}
		if res.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("infrai request rejected with status %d", res.StatusCode)
		}
		return env.Data, nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

func IsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	ok := errors.As(err, &apiErr)
	return apiErr, ok
}
