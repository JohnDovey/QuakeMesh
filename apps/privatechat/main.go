// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 12 reference app: 1:1 private messaging via mesh-sdk Send/Receive.

package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/JohnDovey/QuakeMesh/sdk"
)

const appID = "net.quakemesh.privatechat"

type chatMessage struct {
	Text   string `json:"text"`
	Sender string `json:"sender,omitempty"`
}

func main() {
	socket := flag.String("socket", "/tmp/quakemeshhub.sock", "QuakeMeshHub daemon Unix socket")
	tcp := flag.String("tcp", "", "loopback TCP host:port (overrides -socket)")
	dest := flag.String("dest", "", "destination node id (hex)")
	msg := flag.String("msg", "", "message text to send")
	listen := flag.Bool("listen", false, "receive messages until interrupted")
	discover := flag.Bool("discover", false, "list peers running privatechat")
	flag.Parse()

	client := dialClient(*socket, *tcp)
	sess, err := client.Register(appID, "Private Chat", "0.1.0", []string{"messaging"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "registered as %s on node %s\n", appID, hex.EncodeToString(sess.NodeID))

	if *discover {
		peers, err := client.DiscoverPeers(appID, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: %v\n", err)
			os.Exit(1)
		}
		for _, p := range peers {
			fmt.Println(hex.EncodeToString(p))
		}
		return
	}

	if *dest != "" && *msg != "" {
		destBytes, err := hex.DecodeString(strings.TrimPrefix(*dest, "0x"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid dest: %v\n", err)
			os.Exit(1)
		}
		payload, _ := json.Marshal(chatMessage{
			Text:   *msg,
			Sender: hex.EncodeToString(sess.NodeID),
		})
		if err := client.Send(sess, destBytes, payload); err != nil {
			fmt.Fprintf(os.Stderr, "send: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("sent")
		return
	}

	if *listen {
		ch, err := client.Receive(sess)
		if err != nil {
			fmt.Fprintf(os.Stderr, "receive: %v\n", err)
			os.Exit(1)
		}
		for payload := range ch {
			var m chatMessage
			if err := json.Unmarshal(payload, &m); err != nil {
				fmt.Printf("<raw> %s\n", string(payload))
				continue
			}
			from := m.Sender
			if from == "" {
				from = "?"
			}
			fmt.Printf("[%s] %s\n", from[:min(16, len(from))], m.Text)
		}
		return
	}

	flag.Usage()
	os.Exit(2)
}

func dialClient(socket, tcp string) sdk.Client {
	if tcp != "" {
		return sdk.NewTCPClient(tcp)
	}
	return sdk.NewUnixClient(socket)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
