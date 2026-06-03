// Package infra adapts persistence and execution transports for locations.
//
// Repository adapters persist location records. Transport adapters perform
// local or SSH probes and filesystem listing without exposing credentials
// through domain or RPC DTOs.
package infra
