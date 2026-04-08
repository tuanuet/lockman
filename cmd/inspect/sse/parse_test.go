package sse

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseEvents(t *testing.T) {
	input := "data: {\"kind\":\"acquire_succeeded\"}\n\ndata: {\"kind\":\"contention\"}\n"
	events := make(chan json.RawMessage, 10)
	errors := make(chan error, 10)

	go ParseEvents(strings.NewReader(input), events, errors)

	var got []json.RawMessage
	for e := range events {
		got = append(got, e)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}

	var evt map[string]string
	if err := json.Unmarshal(got[0], &evt); err != nil {
		t.Fatal(err)
	}
	if evt["kind"] != "acquire_succeeded" {
		t.Errorf("event 0 kind = %q", evt["kind"])
	}
}

func TestParseEvents_Malformed(t *testing.T) {
	input := "data: not-json\n"
	events := make(chan json.RawMessage, 10)
	errors := make(chan error, 10)

	go ParseEvents(strings.NewReader(input), events, errors)

	for range events {
	}

	select {
	case err := <-errors:
		if err == nil {
			t.Error("expected non-nil error")
		}
	default:
		t.Error("expected error for malformed JSON")
	}
}

func TestParseEvents_MultiLine(t *testing.T) {
	input := "data: {\"kind\":\"acquire_succeeded\",\ndata: \"definition_id\":\"order\"}\n\n"
	events := make(chan json.RawMessage, 10)
	errors := make(chan error, 10)

	go ParseEvents(strings.NewReader(input), events, errors)

	var got []json.RawMessage
	for e := range events {
		got = append(got, e)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
}
