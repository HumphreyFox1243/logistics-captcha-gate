package logisticsgate

import (
	"testing"
	"time"
)

func TestShipmentStateTransitions(t *testing.T) {
	now := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name              string
		events            []ShipmentEvent
		wantStatus        ShipmentEventType
		wantProof         bool
		wantException     bool
		wantRecordedCount int
	}{
		{
			name: "delivery stores proof and clears exception",
			events: []ShipmentEvent{
				{ShipmentID: "SHP-42", EventID: "evt-1", Type: EventDeliveryException, OccurredAt: now, Exception: &DeliveryException{Code: "ADDRESS_CHECK", Detail: "recipient confirmation required", ReportedAt: now}},
				{ShipmentID: "SHP-42", EventID: "evt-2", Type: EventDelivered, OccurredAt: now.Add(time.Hour), Proof: &ProofOfDelivery{ObjectKey: "pod/SHP-42.jpg", MediaType: "image/jpeg", CapturedAt: now.Add(time.Hour)}},
			},
			wantStatus: EventDelivered, wantProof: true, wantRecordedCount: 2,
		},
		{
			name: "duplicate event is idempotent",
			events: []ShipmentEvent{
				{ShipmentID: "SHP-43", EventID: "evt-3", Type: EventInTransit, OccurredAt: now},
				{ShipmentID: "SHP-43", EventID: "evt-3", Type: EventInTransit, OccurredAt: now},
			},
			wantStatus: EventInTransit, wantRecordedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ledger := NewShipmentLedger()
			var got Shipment
			for _, event := range tt.events {
				var err error
				got, err = ledger.Record(event)
				if err != nil {
					t.Fatal(err)
				}
			}
			if got.Status != tt.wantStatus || (got.Proof != nil) != tt.wantProof || (got.ActiveException != nil) != tt.wantException {
				t.Fatalf("shipment state = %#v", got)
			}
			if len(got.Events) != tt.wantRecordedCount {
				t.Fatalf("event count = %d, want %d", len(got.Events), tt.wantRecordedCount)
			}
		})
	}
}
