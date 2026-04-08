package screens

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/observe"
)

func TestStream_PauseResume(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)

	if s.paused {
		t.Error("expected not paused")
	}

	s.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if !s.paused {
		t.Error("expected paused after Space")
	}

	s.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if s.paused {
		t.Error("expected not paused after second Space")
	}
}

func TestStream_Reconnect(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)
	s.maxRetries = 3

	s.Update(streamErr{fmt.Errorf("connection lost")})
	s.Update(streamErr{fmt.Errorf("connection lost")})
	if s.err != "" {
		t.Error("expected no error after 2 retries")
	}

	s.Update(streamErr{fmt.Errorf("connection lost")})

	if s.err == "" {
		t.Error("expected error message after 3 retries")
	}
	if !strings.Contains(s.err, "press R to reconnect") {
		t.Errorf("unexpected error: %s", s.err)
	}
}

func TestStream_AppendEvents(t *testing.T) {
	c := client.New("http://localhost")
	s := NewStream(c)

	for i := 0; i < 5; i++ {
		s.Update(streamEventMsg{Event: observe.Event{
			Kind:         observe.EventAcquireSucceeded,
			DefinitionID: "order",
			ResourceID:   fmt.Sprintf("order:%d", i),
		}})
	}

	if len(s.events) != 5 {
		t.Errorf("expected 5 events, got %d", len(s.events))
	}
}
