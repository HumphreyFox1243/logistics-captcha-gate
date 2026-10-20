package logisticsgate

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrShipmentNotFound = errors.New("shipment not found")
	ErrInvalidEvent     = errors.New("shipment_id, event_id, type, and occurred_at are required")
)

type ShipmentEventType string

const (
	EventPickedUp          ShipmentEventType = "picked_up"
	EventInTransit         ShipmentEventType = "in_transit"
	EventDelivered         ShipmentEventType = "delivered"
	EventDeliveryException ShipmentEventType = "delivery_exception"
)

type ProofOfDelivery struct {
	ObjectKey  string    `json:"object_key"`
	MediaType  string    `json:"media_type"`
	CapturedAt time.Time `json:"captured_at"`
}

type DeliveryException struct {
	Code       string    `json:"code"`
	Detail     string    `json:"detail"`
	ReportedAt time.Time `json:"reported_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

type ShipmentEvent struct {
	ShipmentID string             `json:"shipment_id"`
	EventID    string             `json:"event_id"`
	Type       ShipmentEventType  `json:"type"`
	OccurredAt time.Time          `json:"occurred_at"`
	Location   string             `json:"location,omitempty"`
	Proof      *ProofOfDelivery   `json:"proof,omitempty"`
	Exception  *DeliveryException `json:"exception,omitempty"`
}

type Shipment struct {
	ID              string             `json:"id"`
	Status          ShipmentEventType  `json:"status"`
	Events          []ShipmentEvent    `json:"events"`
	Proof           *ProofOfDelivery   `json:"proof,omitempty"`
	ActiveException *DeliveryException `json:"active_exception,omitempty"`
}

type ShipmentLedger struct {
	mu        sync.RWMutex
	shipments map[string]Shipment
	seen      map[string]struct{}
}

func NewShipmentLedger() *ShipmentLedger {
	return &ShipmentLedger{shipments: make(map[string]Shipment), seen: make(map[string]struct{})}
}

func (l *ShipmentLedger) Record(event ShipmentEvent) (Shipment, error) {
	if event.ShipmentID == "" || event.EventID == "" || event.Type == "" || event.OccurredAt.IsZero() {
		return Shipment{}, ErrInvalidEvent
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.seen[event.EventID]; exists {
		return l.shipments[event.ShipmentID], nil
	}

	shipment := l.shipments[event.ShipmentID]
	shipment.ID = event.ShipmentID
	shipment.Status = event.Type
	shipment.Events = append(shipment.Events, event)
	if event.Type == EventDelivered && event.Proof != nil {
		proof := *event.Proof
		shipment.Proof = &proof
		shipment.ActiveException = nil
	}
	if event.Type == EventDeliveryException && event.Exception != nil {
		exception := *event.Exception
		shipment.ActiveException = &exception
	}
	l.shipments[event.ShipmentID] = shipment
	l.seen[event.EventID] = struct{}{}
	return shipment, nil
}

func (l *ShipmentLedger) Get(id string) (Shipment, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	shipment, ok := l.shipments[id]
	if !ok {
		return Shipment{}, ErrShipmentNotFound
	}
	return shipment, nil
}
