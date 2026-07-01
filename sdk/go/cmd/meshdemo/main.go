// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 12: exercises every mesh-sdk Client method against a running daemon.

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/JohnDovey/QuakeMesh/sdk"
)

const appID = "net.quakemesh.meshdemo"

func main() {
	socket := flag.String("socket", "/tmp/quakemeshhub.sock", "QuakeMeshHub daemon Unix socket")
	tcp := flag.String("tcp", "", "loopback TCP host:port (overrides -socket)")
	flag.Parse()

	client := dialClient(*socket, *tcp)
	sess, err := client.Register(appID, "Mesh Demo", "0.1.0", []string{"demo"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("register ok node=%s\n", hex.EncodeToString(sess.NodeID))

	peers, err := client.DiscoverPeers(appID, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover-peers: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("discover-peers ok count=%d\n", len(peers))

	subClient := dialClient(*socket, *tcp)
	if _, err := subClient.Register(appID+"-sub", "Mesh Demo Sub", "0.1.0", nil); err != nil {
		fmt.Fprintf(os.Stderr, "register subscriber: %v\n", err)
		os.Exit(1)
	}
	topic := "meshdemo-smoke"
	done := make(chan []byte, 1)
	ch, err := subClient.Subscribe(nil, topic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "subscribe: %v\n", err)
		os.Exit(1)
	}
	go func() {
		select {
		case p := <-ch:
			done <- p
		case <-time.After(5 * time.Second):
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if err := client.Publish(sess, topic, []byte("meshdemo-ping")); err != nil {
		fmt.Fprintf(os.Stderr, "publish: %v\n", err)
		os.Exit(1)
	}
	select {
	case p := <-done:
		fmt.Printf("publish/subscribe ok payload=%q\n", string(p))
	case <-time.After(6 * time.Second):
		fmt.Fprintf(os.Stderr, "subscribe timed out\n")
		os.Exit(1)
	}

	fmt.Println("meshdemo ok")
}

func dialClient(socket, tcp string) *sdk.HTTPClient {
	if tcp != "" {
		return sdk.NewTCPClient(tcp)
	}
	return sdk.NewUnixClient(socket)
}
