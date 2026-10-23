package dynoutbox_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"uuid"

	"github.com/mbarboss/dynoutbox"
)

const (
	validSource     = "com.example.orders"
	validDetailType = "OrderCreated"
)

var validPayload = []byte(`{"orderId":"o-1"}`)

// nested returns a JSON object whose deepest container sits at the given depth,
// counting the root object as depth 1.
func nested(depth int) []byte {
	return []byte(`{"a":` + strings.Repeat("[", depth-1) + strings.Repeat("]", depth-1) + `}`)
}

// sized returns a JSON object of exactly n bytes.
func sized(n int) []byte {
	return []byte(`{"p":"` + strings.Repeat("a", n-8) + `"}`)
}

func TestNewEventReturnsValidatedFields(t *testing.T) {
	at := time.Date(2026, 10, 22, 15, 4, 5, 6, time.FixedZone("BRT", -3*60*60))

	e, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload,
		dynoutbox.WithOccurredAt(at), dynoutbox.WithCorrelationID("req-42"))
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if e.Source() != validSource {
		t.Errorf("Source() = %q, want %q", e.Source(), validSource)
	}
	if e.DetailType() != validDetailType {
		t.Errorf("DetailType() = %q, want %q", e.DetailType(), validDetailType)
	}
	if !bytes.Equal(e.Payload(), validPayload) {
		t.Errorf("Payload() = %s, want %s", e.Payload(), validPayload)
	}
	if !e.OccurredAt().Equal(at) || e.OccurredAt().Location() != time.UTC {
		t.Errorf("OccurredAt() = %v, want %v in UTC", e.OccurredAt(), at.UTC())
	}
	if e.CorrelationID() != "req-42" {
		t.Errorf("CorrelationID() = %q, want %q", e.CorrelationID(), "req-42")
	}
}

func TestNewEventIDIsVersion7(t *testing.T) {
	e, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	id := e.ID()
	if version := id[6] >> 4; version != 7 {
		t.Errorf("ID() version = %d, want 7", version)
	}
	if variant := id[8] >> 6; variant != 0b10 {
		t.Errorf("ID() variant bits = %02b, want 10 (RFC 9562)", variant)
	}
}

func TestNewEventIDsAreUnique(t *testing.T) {
	seen := make(map[uuid.UUID]bool)
	for range 10_000 {
		e, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
		if err != nil {
			t.Fatalf("NewEvent() error = %v", err)
		}
		if seen[e.ID()] {
			t.Fatalf("duplicate ID %s", e.ID())
		}
		seen[e.ID()] = true
	}
}

func TestNewEventIDsSortByCreationTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
		if err != nil {
			t.Fatalf("NewEvent() error = %v", err)
		}
		time.Sleep(2 * time.Millisecond)
		second, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
		if err != nil {
			t.Fatalf("NewEvent() error = %v", err)
		}

		if first.ID().Compare(second.ID()) >= 0 {
			t.Errorf("first ID %s does not sort before second ID %s", first.ID(), second.ID())
		}
	})
}

func TestNewEventDefaultsOccurredAtToNowInUTC(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
		if err != nil {
			t.Fatalf("NewEvent() error = %v", err)
		}

		if want := time.Now(); !e.OccurredAt().Equal(want) || e.OccurredAt().Location() != time.UTC {
			t.Errorf("OccurredAt() = %v, want %v in UTC", e.OccurredAt(), want.UTC())
		}
	})
}

func TestNewEventWithoutCorrelationIDHasEmptyOne(t *testing.T) {
	e, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if e.CorrelationID() != "" {
		t.Errorf("CorrelationID() = %q, want empty", e.CorrelationID())
	}
}

func TestNewEventCopiesPayloadIn(t *testing.T) {
	payload := []byte(`{"a":1}`)
	e, err := dynoutbox.NewEvent(validSource, validDetailType, payload)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	payload[5] = '2'

	if got := string(e.Payload()); got != `{"a":1}` {
		t.Errorf("Payload() = %s after caller mutated its slice, want {\"a\":1}", got)
	}
}

func TestEventPayloadReturnsCopy(t *testing.T) {
	e, err := dynoutbox.NewEvent(validSource, validDetailType, []byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	returned := e.Payload()
	if len(returned) != len(`{"a":1}`) {
		t.Fatalf("Payload() = %s, want {\"a\":1}", returned)
	}
	returned[5] = '2'

	if got := string(e.Payload()); got != `{"a":1}` {
		t.Errorf("Payload() = %s after mutating a returned slice, want {\"a\":1}", got)
	}
}

func TestNewEventAcceptsBoundaryValues(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		detailType string
		payload    []byte
		opts       []dynoutbox.EventOption
	}{
		{name: "source of 256 characters", source: strings.Repeat("a", 256)},
		{name: "source with hyphens and dots", source: "com.my-company.orders-v2"},
		{name: "source starting with aws but not the aws. prefix", source: "awsome.orders"},
		{name: "detail type of 128 characters", detailType: strings.Repeat("A", 128)},
		{name: "detail type with dots, hyphens and underscores", detailType: "order.created_v2-beta"},
		{name: "payload of exactly MaxPayloadBytes", payload: sized(dynoutbox.MaxPayloadBytes)},
		{name: "payload nested exactly 1000 levels", payload: nested(1000)},
		{name: "payload with leading and trailing whitespace", payload: []byte(" \n{\"a\":1}\t ")},
		{name: "payload with escaped quotes and brackets in strings", payload: []byte(`{"a":"\"[{"}`)},
		{name: "payload with non-ASCII UTF-8", payload: []byte(`{"name":"João"}`)},
		{name: "empty object payload", payload: []byte(`{}`)},
		{name: "correlation ID of 128 characters", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID(strings.Repeat("c", 128))}},
		{name: "correlation ID with allowed punctuation", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("1-abc:def_g.h-i")}},
		{name: "correlation ID that is a UUID", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("0199f0c2-8e1a-7b3c-9d4e-5f6a7b8c9d0e")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, detailType, payload := orDefault(tt.source, validSource), orDefault(tt.detailType, validDetailType), tt.payload
			if payload == nil {
				payload = validPayload
			}

			if _, err := dynoutbox.NewEvent(source, detailType, payload, tt.opts...); err != nil {
				t.Errorf("NewEvent() error = %v, want nil", err)
			}
		})
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func TestNewEventRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		source     *string
		detailType *string
		payload    []byte
		nilPayload bool
		opts       []dynoutbox.EventOption
		wantField  string
	}{
		{name: "empty source", source: ptr(""), wantField: "source"},
		{name: "source over 256 characters", source: ptr(strings.Repeat("a", 257)), wantField: "source"},
		{name: "source with uppercase", source: ptr("Com.Example.Orders"), wantField: "source"},
		{name: "source with reserved aws. prefix", source: ptr("aws.orders"), wantField: "source"},
		{name: "source with empty segment", source: ptr("com..orders"), wantField: "source"},
		{name: "source with leading dot", source: ptr(".orders"), wantField: "source"},
		{name: "source with trailing dot", source: ptr("orders."), wantField: "source"},
		{name: "source with space", source: ptr("com example"), wantField: "source"},
		{name: "source with null byte", source: ptr("com.exa\x00mple"), wantField: "source"},
		{name: "source with newline", source: ptr("com.example\n"), wantField: "source"},
		{name: "source with Cyrillic homoglyph", source: ptr("com.exаmple"), wantField: "source"},
		{name: "source with invalid UTF-8", source: ptr("com.\xffexample"), wantField: "source"},
		{name: "empty detail type", detailType: ptr(""), wantField: "detail_type"},
		{name: "detail type over 128 characters", detailType: ptr(strings.Repeat("A", 129)), wantField: "detail_type"},
		{name: "detail type starting with punctuation", detailType: ptr("-OrderCreated"), wantField: "detail_type"},
		{name: "detail type with space", detailType: ptr("Order Created"), wantField: "detail_type"},
		{name: "detail type with tab", detailType: ptr("Order\tCreated"), wantField: "detail_type"},
		{name: "detail type with null byte", detailType: ptr("Order\x00Created"), wantField: "detail_type"},
		{name: "detail type with Cyrillic homoglyph", detailType: ptr("OrderСreated"), wantField: "detail_type"},
		{name: "nil payload", nilPayload: true, wantField: "payload"},
		{name: "empty payload", payload: []byte{}, wantField: "payload"},
		{name: "whitespace payload", payload: []byte("  \n"), wantField: "payload"},
		{name: "null payload", payload: []byte("null"), wantField: "payload"},
		{name: "array payload", payload: []byte(`[{"a":1}]`), wantField: "payload"},
		{name: "string payload", payload: []byte(`"order"`), wantField: "payload"},
		{name: "number payload", payload: []byte(`42`), wantField: "payload"},
		{name: "truncated object", payload: []byte(`{"a":`), wantField: "payload"},
		{name: "two objects", payload: []byte(`{} {}`), wantField: "payload"},
		{name: "trailing garbage", payload: []byte(`{"a":1}x`), wantField: "payload"},
		{name: "invalid UTF-8 in a string", payload: []byte("{\"a\":\"\xff\"}"), wantField: "payload"},
		{name: "payload one byte over MaxPayloadBytes", payload: sized(dynoutbox.MaxPayloadBytes + 1), wantField: "payload"},
		{name: "payload of 10 MB", payload: sized(10 << 20), wantField: "payload"},
		{name: "payload nested 1001 levels", payload: nested(1001), wantField: "payload"},
		{name: "payload nested 100000 levels", payload: nested(100_000), wantField: "payload"},
		{name: "zero occurred-at", opts: []dynoutbox.EventOption{dynoutbox.WithOccurredAt(time.Time{})}, wantField: "occurred_at"},
		{name: "empty correlation ID", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("")}, wantField: "correlation_id"},
		{name: "correlation ID over 128 characters", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID(strings.Repeat("c", 129))}, wantField: "correlation_id"},
		{name: "correlation ID with newline (log injection)", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("req-1\nlevel=ERROR")}, wantField: "correlation_id"},
		{name: "correlation ID with equals sign (breaks key=value logs)", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("Root=1-abc")}, wantField: "correlation_id"},
		{name: "correlation ID with space", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("req 1")}, wantField: "correlation_id"},
		{name: "correlation ID with Cyrillic homoglyph", opts: []dynoutbox.EventOption{dynoutbox.WithCorrelationID("rеq-1")}, wantField: "correlation_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, detailType, payload := validSource, validDetailType, validPayload
			if tt.source != nil {
				source = *tt.source
			}
			if tt.detailType != nil {
				detailType = *tt.detailType
			}
			if tt.payload != nil {
				payload = tt.payload
			}
			if tt.nilPayload {
				payload = nil
			}

			_, err := dynoutbox.NewEvent(source, detailType, payload, tt.opts...)

			if !errors.Is(err, dynoutbox.ErrInvalidEvent) {
				t.Fatalf("NewEvent() error = %v, want ErrInvalidEvent", err)
			}
			var verr *dynoutbox.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("NewEvent() error = %v, want *ValidationError", err)
			}
			if verr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", verr.Field, tt.wantField)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestValidationErrorDoesNotEchoInput(t *testing.T) {
	const secret = "maria.silva@example.com"
	tests := []struct {
		name string
		call func() error
	}{
		{name: "source", call: func() error {
			_, err := dynoutbox.NewEvent(secret, validDetailType, validPayload)
			return err
		}},
		{name: "detail type", call: func() error {
			_, err := dynoutbox.NewEvent(validSource, secret, validPayload)
			return err
		}},
		{name: "payload", call: func() error {
			_, err := dynoutbox.NewEvent(validSource, validDetailType, []byte(`{"email":"`+secret+`"`))
			return err
		}},
		{name: "correlation ID", call: func() error {
			_, err := dynoutbox.NewEvent(validSource, validDetailType, validPayload, dynoutbox.WithCorrelationID(secret+" x"))
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()

			if err == nil {
				t.Fatal("NewEvent() error = nil, want a validation error")
			}
			if strings.Contains(err.Error(), "maria") {
				t.Errorf("error %q echoes the input", err)
			}
		})
	}
}

func TestValidationErrorMessageNamesFieldAndReason(t *testing.T) {
	err := &dynoutbox.ValidationError{Field: "source", Reason: "must not be empty"}

	if got, want := err.Error(), "dynoutbox: invalid event: source: must not be empty"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func ExampleNewEvent() {
	e, err := dynoutbox.NewEvent("com.example.orders", "OrderCreated", []byte(`{"orderId":"o-1"}`),
		dynoutbox.WithCorrelationID("req-42"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(e.Source(), e.DetailType(), string(e.Payload()), e.CorrelationID())
	// Output: com.example.orders OrderCreated {"orderId":"o-1"} req-42
}
