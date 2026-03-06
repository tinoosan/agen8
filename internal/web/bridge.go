package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// handleRPC forwards a single JSON-RPC request from the browser (HTTP POST body)
// to the daemon TCP socket and writes the response back.
// Protocol: open fresh TCP connection, write request JSON, close write half,
// read until we see a response whose id matches the request id.
func handleRPC(w http.ResponseWriter, r *http.Request, rpcEndpoint string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse out the request id so we can match the response.
	var req struct {
		ID *json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json-rpc request: "+err.Error(), http.StatusBadRequest)
		return
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(r.Context(), "tcp", rpcEndpoint)
	if err != nil {
		http.Error(w, "connect daemon: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	// Write request then close the write half so the daemon knows it's done.
	if _, werr := conn.Write(body); werr != nil {
		http.Error(w, "send to daemon: "+werr.Error(), http.StatusBadGateway)
		return
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}

	// Derive the target id as a plain string for comparison.
	var reqID string
	if req.ID != nil {
		reqID = strings.Trim(string(*req.ID), `"`)
	}

	// Read JSON objects from the TCP stream, skip notifications (no id), return
	// the first response whose id matches.
	dec := json.NewDecoder(conn)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	for {
		var msg json.RawMessage
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				http.Error(w, "daemon closed connection without response", http.StatusBadGateway)
				return
			}
			http.Error(w, "decode daemon response: "+err.Error(), http.StatusBadGateway)
			return
		}
		// Quick check: extract the id field.
		var envelope struct {
			ID *json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}
		if envelope.ID == nil {
			// Notification — skip, we don't forward these here.
			continue
		}
		msgID := strings.Trim(string(*envelope.ID), `"`)
		if msgID != reqID {
			continue
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write(msg)
		return
	}
}

// handleEvents opens a persistent TCP connection to the daemon and streams
// notifications (messages with no id) back to the browser as Server-Sent Events.
func handleEvents(w http.ResponseWriter, r *http.Request, rpcEndpoint string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(r.Context(), "tcp", rpcEndpoint)
	if err != nil {
		http.Error(w, "connect daemon: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer conn.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send a keepalive immediately so the browser knows we're connected.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	doneCh := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-doneCh:
		}
	}()
	defer close(doneCh)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Only forward notifications (id == null / missing).
		var envelope struct {
			ID *json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}
		if envelope.ID != nil {
			// Response to a request — not a notification, skip.
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()

		if ctx.Err() != nil {
			return
		}
	}
}
