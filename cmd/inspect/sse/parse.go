package sse

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// ParseEvents reads an SSE stream and sends parsed JSON raw messages to the events channel.
// It accumulates multi-line "data:" fields into a single payload before emitting.
// Errors are sent to the errors channel. Both channels are closed when the stream ends.
func ParseEvents(r io.Reader, events chan<- json.RawMessage, errors chan<- error) {
	defer close(events)
	defer close(errors)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*64), 1024*64)

	var dataLines []string
	flushData := func() {
		if len(dataLines) == 0 {
			return
		}
		combined := strings.Join(dataLines, "\n")
		dataLines = nil
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(combined), &raw); err != nil {
			errors <- err
			return
		}
		events <- raw
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushData()
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, line[6:])
		}
		// Ignore other SSE fields (id, event, retry)
	}
	flushData()
	if err := scanner.Err(); err != nil {
		errors <- err
	}
}
