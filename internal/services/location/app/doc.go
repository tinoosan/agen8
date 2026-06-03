// Package app coordinates location-service use cases.
//
// The app layer validates client intent, calls transports for probes and
// filesystem browsing, persists sanitized location state, and exposes views for
// RPC handlers. It does not store credentials or start harness sessions.
package app
