package protocol

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestRunNotificationListener_ContextCancelUnblocksIdleRead(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		accepted <- c
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunNotificationListener(ctx, ln.Addr().String(), func(msg Message) error { return nil })
	}()

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not connect")
	}
	defer serverConn.Close()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not return after context cancellation")
	}
}
