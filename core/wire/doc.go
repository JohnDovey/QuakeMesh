// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial scaffold: go:generate directive wiring /proto into
//           /core/wire.

// Package wire holds generated Go types for the /proto schemas. Do not
// edit files in this directory by hand -- regenerate with:
//
//	go generate ./...
//
//go:generate protoc --go_out=. --go_opt=paths=source_relative --proto_path=../../proto ../../proto/frame.proto ../../proto/ogm.proto ../../proto/dtn.proto ../../proto/trust.proto ../../proto/relay_hub.proto ../../proto/hub_sync.proto ../../proto/app_presence.proto ../../proto/ban_list.proto ../../proto/management.proto
package wire
