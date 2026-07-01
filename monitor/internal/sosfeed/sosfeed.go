// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.15 - Phase 14: in-memory ring buffer of recent SOS alerts.

package sosfeed

import (
	"encoding/json"
	"sync"
	"time"
)

const maxAlerts = 100

// Alert is one decoded SOS publish event for the Monitor dashboard.
type Alert struct {
	ReceivedAt  time.Time `json:"received_at"`
	NodeID      string    `json:"node_id"`
	AppID       string    `json:"app_id"`
	Topic       string    `json:"topic"`
	Text        string    `json:"text"`
	Lat         float64   `json:"lat,omitempty"`
	Lon         float64   `json:"lon,omitempty"`
	AccuracyM   float64   `json:"accuracy_m,omitempty"`
	SentAtUnix  int64     `json:"sent_at_unix_ms,omitempty"`
	RawPayload  string    `json:"raw_payload,omitempty"`
}

// Store retains the most recent SOS alerts in memory.
type Store struct {
	mu     sync.RWMutex
	alerts []Alert
}

// New creates an empty SOS feed store.
func New() *Store {
	return &Store{}
}

// Add records an SOS alert from a hub management event.
func (s *Store) Add(nodeID, appID, topic string, payload []byte) Alert {
	var parsed struct {
		Text      string  `json:"text"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		AccuracyM float64 `json:"accuracy_m"`
		SentAt    int64   `json:"sent_at"`
	}
	raw := string(payload)
	_ = json.Unmarshal(payload, &parsed)
	alert := Alert{
		ReceivedAt: time.Now().UTC(),
		NodeID:     nodeID,
		AppID:      appID,
		Topic:      topic,
		Text:       parsed.Text,
		Lat:        parsed.Lat,
		Lon:        parsed.Lon,
		AccuracyM:  parsed.AccuracyM,
		SentAtUnix: parsed.SentAt,
	}
	if alert.Text == "" {
		alert.RawPayload = raw
	}
	s.mu.Lock()
	s.alerts = append([]Alert{alert}, s.alerts...)
	if len(s.alerts) > maxAlerts {
		s.alerts = s.alerts[:maxAlerts]
	}
	s.mu.Unlock()
	return alert
}

// List returns alerts newest-first.
func (s *Store) List() []Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Alert, len(s.alerts))
	copy(out, s.alerts)
	return out
}
