// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.15 - Phase 14 SOS feed store tests.

package sosfeed

import (
	"testing"
	"time"
)

func TestStore_AddAndList(t *testing.T) {
	s := New()
	payload := []byte(`{"text":"help","lat":-36.8,"lon":174.7,"accuracy_m":10,"sent_at":123}`)
	a := s.Add("node1", "net.quakemesh.sosbeacon", "sos", payload)
	if a.Text != "help" || a.Lat != -36.8 {
		t.Fatalf("alert = %+v", a)
	}
	list := s.List()
	if len(list) != 1 || list[0].NodeID != "node1" {
		t.Fatalf("list = %+v", list)
	}
	s.Add("node2", "net.quakemesh.sosbeacon", "sos", []byte(`{"text":"second"}`))
	list = s.List()
	if len(list) != 2 || list[0].Text != "second" {
		t.Fatalf("newest first: %+v", list)
	}
}

func TestStore_Cap(t *testing.T) {
	s := New()
	for i := 0; i < maxAlerts+5; i++ {
		s.Add("n", "app", "sos", []byte(`{"text":"x"}`))
		time.Sleep(time.Millisecond)
	}
	if len(s.List()) != maxAlerts {
		t.Fatalf("cap = %d", len(s.List()))
	}
}
