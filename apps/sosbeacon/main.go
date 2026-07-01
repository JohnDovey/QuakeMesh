// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 13 reference app: emergency SOS beacon via Publish/Subscribe.

package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/JohnDovey/QuakeMesh/sdk"
)

const (
	appID = "net.quakemesh.sosbeacon"
	topic = "sos"
)

type sosAlert struct {
	Text      string  `json:"text"`
	Lat       float64 `json:"lat,omitempty"`
	Lon       float64 `json:"lon,omitempty"`
	AccuracyM float64 `json:"accuracy_m,omitempty"`
	NodeID    string  `json:"node_id,omitempty"`
	SentAt    int64   `json:"sent_at"`
}

func main() {
	socket := flag.String("socket", "/tmp/quakemeshhub.sock", "QuakeMeshHub daemon Unix socket")
	tcp := flag.String("tcp", "", "loopback TCP host:port (overrides -socket)")
	text := flag.String("text", "SOS — need assistance", "alert message")
	post := flag.String("post", "", "alias for -text (same as discuss app)")
	lat := flag.Float64("lat", 0, "latitude (optional)")
	lon := flag.Float64("lon", 0, "longitude (optional)")
	acc := flag.Float64("acc", 0, "accuracy in metres (optional)")
	listen := flag.Bool("listen", false, "subscribe and print SOS alerts until interrupted")
	flag.Parse()

	msg := *text
	if *post != "" {
		msg = *post
	}

	client := dialClient(*socket, *tcp)
	sess, err := client.Register(appID, "SOS Beacon", "0.1.0", []string{"sos", "location"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "registered as %s on node %s\n", appID, hex.EncodeToString(sess.NodeID))

	if *listen {
		ch, err := client.Subscribe(sess, topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "subscribe: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "listening for SOS alerts on topic %q\n", topic)
		for payload := range ch {
			printAlert(payload)
		}
		return
	}

	alert := sosAlert{
		Text:   msg,
		Lat:    *lat,
		Lon:    *lon,
		NodeID: hex.EncodeToString(sess.NodeID),
		SentAt: time.Now().UnixMilli(),
	}
	if *acc > 0 {
		alert.AccuracyM = *acc
	}
	payload, _ := json.Marshal(alert)
	if err := client.Publish(sess, topic, payload); err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("sos published")
}

func printAlert(payload []byte) {
	var a sosAlert
	if err := json.Unmarshal(payload, &a); err != nil {
		fmt.Printf("<raw> %s\n", string(payload))
		return
	}
	when := time.UnixMilli(a.SentAt).Format(time.RFC3339)
	node := a.NodeID
	if len(node) > 16 {
		node = node[:16] + "…"
	}
	if a.Lat != 0 || a.Lon != 0 {
		fmt.Printf("[%s] SOS from %s @ %.5f,%.5f (±%.0fm): %s\n",
			when, node, a.Lat, a.Lon, a.AccuracyM, a.Text)
		return
	}
	fmt.Printf("[%s] SOS from %s: %s\n", when, node, a.Text)
}

func dialClient(socket, tcp string) sdk.Client {
	if tcp != "" {
		return sdk.NewTCPClient(tcp)
	}
	return sdk.NewUnixClient(socket)
}
