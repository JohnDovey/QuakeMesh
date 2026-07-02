// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - LAN WiFi context detection and segment key helpers.

// Package lancontext provides LAN/WiFi context types and host detection
// used by hub heartbeat, LAN beacons, and infrastructure segments.
package lancontext

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// Context describes a device's attachment to a local Wi-Fi segment.
type Context struct {
	GatewayIP string `json:"gateway_ip"`
	LocalIP   string `json:"local_ip,omitempty"`
	SSID      string `json:"ssid,omitempty"`
	BSSID     string `json:"bssid,omitempty"`
}

// Valid reports whether the context is sufficient to identify a segment.
func (c Context) Valid() bool {
	return c != Context{} && c.GatewayIP != ""
}

// SegmentID returns a deterministic id for gateway_ip + ssid + bssid.
// BSSID is included in the key when present to distinguish multi-AP SSIDs.
func SegmentID(c Context) string {
	key := c.GatewayIP + "|" + c.SSID + "|" + c.BSSID
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

// DetectLocalIP returns the primary outbound IPv4 address, if any.
func DetectLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	return addr.IP.String()
}

// DetectGateway returns the default-route gateway on darwin/linux when known.
func DetectGateway() string {
	switch runtime.GOOS {
	case "linux":
		if gw := gatewayFromIPRoute(); gw != "" {
			return gw
		}
	case "darwin":
		if gw := gatewayFromRouteGet(); gw != "" {
			return gw
		}
	}
	return ""
}

// Detect builds gateway and local IP for this host.
func Detect() Context {
	gw := DetectGateway()
	if gw == "" {
		return Context{}
	}
	return Context{
		GatewayIP: gw,
		LocalIP:   DetectLocalIP(),
	}
}

// LocalIPFromRemoteAddr extracts the client IP from an HTTP RemoteAddr.
func LocalIPFromRemoteAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return strings.TrimSpace(addr)
	}
	return host
}

func gatewayFromIPRoute() string {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "via" && i+1 < len(fields) {
				if ip := net.ParseIP(fields[i+1]); ip != nil && ip.To4() != nil {
					return ip.String()
				}
			}
		}
	}
	return ""
}

func gatewayFromRouteGet() string {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "gateway:") {
			gw := strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
			if ip := net.ParseIP(gw); ip != nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}
