package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	logisticsgate "example.com/logistics-captcha-gate"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		logger.Error("INFRAI_API_KEY is required")
		os.Exit(1)
	}

	client := logisticsgate.NewInfraiClient(apiKey, &http.Client{Timeout: 10 * time.Second})
	handler := logisticsgate.NewHTTPHandler(
		logisticsgate.NewSignupGate(client),
		logisticsgate.NewShipmentLedger(),
		logger,
	)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("logistics gate listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
