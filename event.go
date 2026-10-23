package dynoutbox

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

// MaxPayloadBytes is the largest payload NewEvent accepts. It leaves room for the
// outbox item's metadata under DynamoDB's 400 KB item limit, fits one PutEvents
// entry, and fits an SQS dead-letter message together with the failure reason.
const MaxPayloadBytes = 256 << 10

// EventBridge rejects Detail nested deeper than this.
const maxPayloadDepth = 1000

const (
	maxSourceLen        = 256
	maxDetailTypeLen    = 128
	maxCorrelationIDLen = 128
)

// The patterns are ASCII-only on purpose: they keep homoglyphs, control characters
// and log-breaking bytes out of values that end up in routing rules and log lines.
var (
	sourcePattern        = regexp.MustCompile(`^[a-z0-9]+([.-][a-z0-9]+)*$`)
	detailTypePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	correlationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

// Event is an outbox event. The only way to get one is NewEvent, so every Event in
// the program has passed validation.
type Event struct {
	id            uuid.UUID
	source        string
	detailType    string
	payload       []byte
	occurredAt    time.Time
	correlationID string
}

// EventOption sets an optional field in NewEvent.
type EventOption func(*eventOptions)

type eventOptions struct {
	occurredAt       time.Time
	occurredAtSet    bool
	correlationID    string
	correlationIDSet bool
}

// WithOccurredAt sets when the event happened. Without it, NewEvent uses the current
// time. The value is stored in UTC.
func WithOccurredAt(t time.Time) EventOption {
	return func(o *eventOptions) {
		o.occurredAt = t
		o.occurredAtSet = true
	}
}

// WithCorrelationID links the event to the request or workflow that caused it. It
// appears in log lines, so it allows only ASCII letters, digits and . _ : -
func WithCorrelationID(id string) EventOption {
	return func(o *eventOptions) {
		o.correlationID = id
		o.correlationIDSet = true
	}
}

// NewEvent validates its inputs and returns an Event with a new UUIDv7 ID.
//
// source is a lowercase dotted name such as "com.example.orders" and must not start
// with "aws.", which EventBridge reserves. detailType names the kind of event, such
// as "OrderCreated". payload must be a JSON object of at most MaxPayloadBytes; it
// is copied, so the caller may reuse the slice.
//
// A validation failure returns a *ValidationError that matches ErrInvalidEvent. The
// error names the field and the rule, never the rejected value.
func NewEvent(source, detailType string, payload []byte, opts ...EventOption) (Event, error) {
	var o eventOptions
	for _, opt := range opts {
		opt(&o)
	}
	if err := validateSource(source); err != nil {
		return Event{}, err
	}
	if err := validateDetailType(detailType); err != nil {
		return Event{}, err
	}
	if err := validatePayload(payload); err != nil {
		return Event{}, err
	}
	occurredAt := time.Now()
	if o.occurredAtSet {
		if o.occurredAt.IsZero() {
			return Event{}, invalid("occurred_at", "must not be the zero time")
		}
		occurredAt = o.occurredAt
	}
	if o.correlationIDSet {
		if err := validateCorrelationID(o.correlationID); err != nil {
			return Event{}, err
		}
	}
	return Event{
		id:            uuid.NewV7(),
		source:        source,
		detailType:    detailType,
		payload:       bytes.Clone(payload),
		occurredAt:    occurredAt.UTC(),
		correlationID: o.correlationID,
	}, nil
}

func validateSource(s string) error {
	switch {
	case s == "":
		return invalid("source", "must not be empty")
	case len(s) > maxSourceLen:
		return invalid("source", "must be at most 256 characters")
	case !sourcePattern.MatchString(s):
		return invalid("source", "must be lowercase letters and digits in segments separated by . or -")
	case strings.HasPrefix(s, "aws."):
		return invalid("source", `must not start with the reserved prefix "aws."`)
	}
	return nil
}

func validateDetailType(s string) error {
	switch {
	case s == "":
		return invalid("detail_type", "must not be empty")
	case len(s) > maxDetailTypeLen:
		return invalid("detail_type", "must be at most 128 characters")
	case !detailTypePattern.MatchString(s):
		return invalid("detail_type", "must start with a letter or digit and contain only letters, digits and . _ -")
	}
	return nil
}

func validateCorrelationID(s string) error {
	switch {
	case s == "":
		return invalid("correlation_id", "must not be empty when set")
	case len(s) > maxCorrelationIDLen:
		return invalid("correlation_id", "must be at most 128 characters")
	case !correlationIDPattern.MatchString(s):
		return invalid("correlation_id", "must contain only letters, digits and . _ : -")
	}
	return nil
}

// validatePayload checks the size first so that oversized input is rejected before
// any parsing work.
func validatePayload(p []byte) error {
	switch {
	case len(p) > MaxPayloadBytes:
		return invalid("payload", "must be at most 262144 bytes")
	case !utf8.Valid(p):
		return invalid("payload", "must be valid UTF-8")
	case !json.Valid(p):
		return invalid("payload", "must be a single valid JSON value")
	case bytes.TrimLeft(p, " \t\r\n")[0] != '{':
		return invalid("payload", "must be a JSON object")
	case exceedsDepth(p, maxPayloadDepth):
		return invalid("payload", "must not nest objects or arrays more than 1000 levels deep")
	}
	return nil
}

// exceedsDepth reports whether valid JSON nests objects and arrays deeper than limit.
// It only tracks string boundaries, because json.Valid has already checked syntax.
func exceedsDepth(p []byte, limit int) bool {
	depth := 0
	inString, escaped := false, false
	for _, c := range p {
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case inString:
			inString = c != '"'
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
			if depth > limit {
				return true
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return false
}

// ID returns the event's UUIDv7. IDs sort by creation time.
func (e Event) ID() uuid.UUID { return e.id }

// Source returns the event source, such as "com.example.orders".
func (e Event) Source() string { return e.source }

// DetailType returns the kind of event, such as "OrderCreated".
func (e Event) DetailType() string { return e.detailType }

// Payload returns a copy of the JSON payload.
func (e Event) Payload() []byte { return bytes.Clone(e.payload) }

// OccurredAt returns when the event happened, in UTC.
func (e Event) OccurredAt() time.Time { return e.occurredAt }

// CorrelationID returns the correlation ID, or "" if none was set.
func (e Event) CorrelationID() string { return e.correlationID }
