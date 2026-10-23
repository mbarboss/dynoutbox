package dynoutbox_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mbarboss/dynoutbox"
)

func FuzzNewEvent(f *testing.F) {
	f.Add(validSource, validDetailType, validPayload, "req-42")
	f.Add("aws.orders", "Order Created", []byte(`{} {}`), "req\n1")
	f.Add("com.example", "X", []byte(`{"a":[[[[]]]]}`), "c")
	f.Add("", "", []byte("{\"a\":\"\xff\"}"), "")

	f.Fuzz(func(t *testing.T, source, detailType string, payload []byte, correlationID string) {
		e, err := dynoutbox.NewEvent(source, detailType, payload, dynoutbox.WithCorrelationID(correlationID))
		if err != nil {
			var verr *dynoutbox.ValidationError
			if !errors.Is(err, dynoutbox.ErrInvalidEvent) || !errors.As(err, &verr) {
				t.Fatalf("NewEvent() error = %v, want *ValidationError matching ErrInvalidEvent", err)
			}
			return
		}

		if !json.Valid(e.Payload()) || bytes.TrimSpace(e.Payload())[0] != '{' || len(e.Payload()) > dynoutbox.MaxPayloadBytes {
			t.Fatalf("accepted payload breaks the invariants: %q", e.Payload())
		}
		again, err := dynoutbox.NewEvent(e.Source(), e.DetailType(), e.Payload(), dynoutbox.WithCorrelationID(e.CorrelationID()))
		if err != nil {
			t.Fatalf("accepted fields fail validation on a second pass: %v", err)
		}
		if again.Source() != source || again.DetailType() != detailType || again.CorrelationID() != correlationID {
			t.Fatal("accepted fields changed on a second pass")
		}
	})
}
