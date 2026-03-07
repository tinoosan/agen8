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
// If projectRoot is non-empty, it is injected into the request params so that
// project-scoped RPC methods can resolve the correct workspace.
func handleRPC(w http.ResponseWriter, r *http.Request, rpcEndpoint, projectRoot string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse out the request id so we can match the response.
	var req struct {
		ID     *json.RawMessage `json:"id"`
		Params json.RawMessage  `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json-rpc request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Inject projectRoot into params if the server has one configured and
	// the request doesn't already provide cwd or projectRoot.
	if projectRoot != "" {
		body = injectProjectRoot(body, req.Params, projectRoot)
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

// injectProjectRoot adds "projectRoot" to the JSON-RPC params object if the
// caller hasn't already provided "cwd" or "projectRoot". This allows
// project-scoped daemon methods to locate the correct .agen8/ workspace when
// requests originate from the web UI (which doesn't know the project directory).
func injectProjectRoot(body []byte, rawParams json.RawMessage, projectRoot string) []byte {
	// If params is missing or null, create a new object with projectRoot.
	if len(rawParams) == 0 || string(rawParams) == "null" {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			return body
		}
		p, _ := json.Marshal(map[string]string{"projectRoot": projectRoot})
		envelope["params"] = p
		out, err := json.Marshal(envelope)
		if err != nil {
			return body
		}
		return out
	}

	// Parse existing params.
	var params map[string]json.RawMessage
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return body // params is not an object (e.g. array) — leave unchanged
	}

	// Don't overwrite if cwd or projectRoot is already set.
	if _, ok := params["cwd"]; ok {
		return body
	}
	if _, ok := params["projectRoot"]; ok {
		return body
	}

	// Inject projectRoot into params and rebuild the full request.
	prBytes, _ := json.Marshal(projectRoot)
	params["projectRoot"] = prBytes
	newParams, err := json.Marshal(params)
	if err != nil {
		return body
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	envelope["params"] = newParams
	out, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return out
}
