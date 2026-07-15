// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN beacon encode/decode tests.

package lanbeacon

import (
	"bytes"
	"testing"
)

func TestHubBeaconRoundTrip(t *testing.T) {
	raw, err := HubBeacon("abc123", 18085, 47222, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, Prefix()) {
		t.Fatalf("missing prefix: %q", raw[:5])
	}
	msg, ok, err := Decode(raw)
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if msg.Kind != KindHub || msg.HeartbeatPort != 18085 || msg.OGMPort != 47222 {
		t.Fatalf("msg = %+v", msg)
	}
}

func TestDecodeIgnoresNonBeacon(t *testing.T) {
	_, ok, err := Decode([]byte{0x01, 0x02, 0x03})
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestNodeBeaconLocation(t *testing.T) {
	lat, lon, acc := -36.85, 174.76, 12.0
	raw, err := NodeBeacon("deadbeef", "", nil, nil, &lat, &lon, &acc, nil)
	if err != nil {
		t.Fatal(err)
	}
	msg, ok, err := Decode(raw)
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if msg.Lat == nil || *msg.Lat != lat {
		t.Fatalf("lat = %v", msg.Lat)
	}
}
