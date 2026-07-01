// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Phase 12 reference app: topic-based bulletin board via Publish/Subscribe.

package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/JohnDovey/QuakeMesh/sdk"
)

const appID = "net.quakemesh.discuss"

type boardPost struct {
	Topic  string `json:"topic"`
	Text   string `json:"text"`
	Author string `json:"author,omitempty"`
}

func main() {
	socket := flag.String("socket", "/tmp/quakemeshhub.sock", "QuakeMeshHub daemon Unix socket")
	tcp := flag.String("tcp", "", "loopback TCP host:port (overrides -socket)")
	topic := flag.String("topic", "general", "bulletin topic")
	post := flag.String("post", "", "post text to publish")
	listen := flag.Bool("listen", false, "subscribe and print posts until interrupted")
	flag.Parse()

	client := dialClient(*socket, *tcp)
	sess, err := client.Register(appID, "Discuss", "0.1.0", []string{"pubsub"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "register: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "registered as %s on node %s\n", appID, hex.EncodeToString(sess.NodeID))

	if *post != "" {
		payload, _ := json.Marshal(boardPost{
			Topic:  *topic,
			Text:   *post,
			Author: hex.EncodeToString(sess.NodeID),
		})
		if err := client.Publish(sess, *topic, payload); err != nil {
			fmt.Fprintf(os.Stderr, "publish: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("published")
		return
	}

	if *listen {
		ch, err := client.Subscribe(sess, *topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "subscribe: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "listening on topic %q\n", *topic)
		for payload := range ch {
			var p boardPost
			if err := json.Unmarshal(payload, &p); err != nil {
				fmt.Printf("<raw> %s\n", string(payload))
				continue
			}
			author := p.Author
			if author == "" {
				author = "?"
			}
			if len(author) > 16 {
				author = author[:16]
			}
			fmt.Printf("[%s] %s: %s\n", author, p.Topic, p.Text)
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
