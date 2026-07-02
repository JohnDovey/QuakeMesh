// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - LAN context segment id and helper tests.

package lancontext

import "testing"

func TestSegmentID_Deterministic(t *testing.T) {
	ctx := Context{GatewayIP: "192.168.1.1", SSID: "Home", BSSID: "aa:bb:cc:dd:ee:ff"}
	id1 := SegmentID(ctx)
	id2 := SegmentID(ctx)
	if id1 != id2 || len(id1) != 32 {
		t.Fatalf("segment id = %q", id1)
	}
}

func TestSegmentID_BSSIDDistinguishesSameSSID(t *testing.T) {
	a := SegmentID(Context{GatewayIP: "10.0.0.1", SSID: "Mesh", BSSID: "11:22:33:44:55:66"})
	b := SegmentID(Context{GatewayIP: "10.0.0.1", SSID: "Mesh", BSSID: "aa:bb:cc:dd:ee:ff"})
	if a == b {
		t.Fatalf("expected different segment ids for different BSSIDs")
	}
}

func TestSegmentID_SameWithoutBSSID(t *testing.T) {
	a := SegmentID(Context{GatewayIP: "10.0.0.1", SSID: "Mesh"})
	b := SegmentID(Context{GatewayIP: "10.0.0.1", SSID: "Mesh", BSSID: ""})
	if a != b {
		t.Fatalf("empty BSSID should not change key: %q vs %q", a, b)
	}
}

func TestLocalIPFromRemoteAddr(t *testing.T) {
	if got := LocalIPFromRemoteAddr("192.168.1.42:54321"); got != "192.168.1.42" {
		t.Fatalf("got %q", got)
	}
}

func TestContextValid(t *testing.T) {
	var empty Context
	if empty.Valid() {
		t.Fatal("empty context should be invalid")
	}
	ctx := Context{GatewayIP: "1.2.3.4"}
	if !ctx.Valid() {
		t.Fatal("gateway should be sufficient")
	}
}
