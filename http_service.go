package logisticsgate

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type HTTPService struct {
	gate   *SignupGate
	ledger *ShipmentLedger
	log    *slog.Logger
}

func NewHTTPHandler(gate *SignupGate, ledger *ShipmentLedger, logger *slog.Logger) http.Handler {
	service := &HTTPService{gate: gate, ledger: ledger, log: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", service.signup)
	mux.HandleFunc("POST /shipments/{id}/events", service.recordEvent)
	mux.HandleFunc("GET /shipments/{id}", service.getShipment)
	return requestLog(logger, mux)
}

func (s *HTTPService) signup(w http.ResponseWriter, r *http.Request) {
	var input SignupInput
	if err := decodeJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	input.IP = clientIP(r)
	result, err := s.gate.Register(r.Context(), input)
	if err != nil {
		status := http.StatusBadRequest
		if apiErr, ok := IsAPIError(err); ok {
			switch {
			case apiErr.StatusCode == http.StatusTooManyRequests:
				status = http.StatusTooManyRequests
			case apiErr.StatusCode >= http.StatusInternalServerError:
				status = http.StatusBadGateway
			}
		} else if !errors.Is(err, ErrInvalidSignup) {
			status = http.StatusBadGateway
		}
		s.log.Info("signup denied", "email", input.Email, "status", status)
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	s.log.Info("signup accepted", "email", input.Email)
	writeJSON(w, http.StatusCreated, result)
}

func (s *HTTPService) recordEvent(w http.ResponseWriter, r *http.Request) {
	var event ShipmentEvent
	if err := decodeJSON(r, &event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	event.ShipmentID = r.PathValue("id")
	shipment, err := s.ledger.Record(event)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.log.Info("shipment event recorded", "shipment_id", shipment.ID, "status", shipment.Status)
	writeJSON(w, http.StatusAccepted, shipment)
}

func (s *HTTPService) getShipment(w http.ResponseWriter, r *http.Request) {
	shipment, err := s.ledger.Get(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, shipment)
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds())
	})
}
