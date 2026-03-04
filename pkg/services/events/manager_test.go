package events

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestService_AppendAndListPaginated(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, run, err := implstore.CreateSession(cfg, "Events Service Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	svc := NewService(cfg)

	err = svc.Append(context.Background(), types.EventRecord{
		RunID:   run.RunID,
		Type:    "test_event",
		Message: "hello",
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	err = svc.Append(context.Background(), types.EventRecord{
		RunID:   run.RunID,
		Type:    "test_event",
		Message: "world",
	})
	if err != nil {
		t.Fatalf("Append second: %v", err)
	}

	evs, next, err := svc.ListPaginated(context.Background(), Filter{
		RunID: run.RunID,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evs))
	}
	if next == 0 {
		t.Fatalf("expected non-zero next cursor")
	}
	if evs[0].Message != "hello" || evs[1].Message != "world" {
		t.Errorf("event messages: got %q, %q", evs[0].Message, evs[1].Message)
	}
}

func TestService_CountAndLatestSeq(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, run, err := implstore.CreateSession(cfg, "Count Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	svc := NewService(cfg)

	seq, err := svc.LatestSeq(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq != 0 {
		t.Fatalf("expected seq 0 for no events, got %d", seq)
	}

	for i := 0; i < 3; i++ {
		_ = svc.Append(context.Background(), types.EventRecord{
			RunID:   run.RunID,
			Type:    "info",
			Message: fmt.Sprintf("msg %d", i),
		})
	}

	count, err := svc.Count(context.Background(), Filter{RunID: run.RunID})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}

	seq2, err := svc.LatestSeq(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq2 <= 0 {
		t.Fatalf("expected non-zero seq after appends")
	}
}

func TestService_Tail(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, run, err := implstore.CreateSession(cfg, "Tail Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	svc := NewService(cfg)

	_ = svc.Append(context.Background(), types.EventRecord{RunID: run.RunID, Type: "a", Message: "first"})
	_ = svc.Append(context.Background(), types.EventRecord{RunID: run.RunID, Type: "a", Message: "second"})
	off, _ := svc.LatestSeq(context.Background(), run.RunID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tailCh, errCh := svc.Tail(ctx, run.RunID, off)
	var got []TailedEvent
	for te := range tailCh {
		got = append(got, te)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Tail errCh: %v", err)
		}
	default:
	}
	if len(got) != 0 {
		t.Logf("tail returned %d events (ok if no new events)", len(got))
	}
	cancel()
}

func TestService_Tail_CancelUnblocksWhenConsumerStops(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, run, err := implstore.CreateSession(cfg, "Tail Cancel Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	svc := NewService(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	tailCh, _ := svc.Tail(ctx, run.RunID, 0)

	if err := svc.Append(context.Background(), types.EventRecord{
		RunID:   run.RunID,
		Type:    "test_event",
		Message: "first event",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Consume exactly one event so the consumer is known to be active.
	select {
	case _, ok := <-tailCh:
		if !ok {
			t.Fatalf("tail channel closed before first event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail channel did not deliver first event")
	}

	// Append another event, then stop consuming and cancel; the stream should close promptly.
	if err := svc.Append(context.Background(), types.EventRecord{
		RunID:   run.RunID,
		Type:    "test_event",
		Message: "second event",
	}); err != nil {
		t.Fatalf("Append second: %v", err)
	}
	cancel()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-tailCh:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatal("tail channel did not close after cancellation")
		}
	}
}

func TestService_AppendEvent_StoreAppender(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, run, err := implstore.CreateSession(cfg, "StoreAppender Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	svc := NewService(cfg)

	err = svc.AppendEvent(context.Background(), types.EventRecord{
		RunID:   run.RunID,
		Type:    "store_appender",
		Message: "via AppendEvent",
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	evs, _, _ := svc.ListPaginated(context.Background(), Filter{RunID: run.RunID, Limit: 10})
	if len(evs) != 1 || evs[0].Message != "via AppendEvent" {
		t.Fatalf("expected one event via AppendEvent, got %+v", evs)
	}
}

func TestService_LatestSeq_EmptyRunID(t *testing.T) {
	svc := NewService(config.Config{DataDir: t.TempDir()})
	_, err := svc.LatestSeq(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty runID")
	}
	if !errors.Is(err, ErrRunIDRequired) {
		t.Errorf("expected ErrRunIDRequired, got %v", err)
	}
}
