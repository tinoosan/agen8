package app

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/events"
)

func TestEmitCodeExecProvisioningSecurityWarning_EmitsWhenEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()

	var emitted []events.Event
	emitCodeExecProvisioningSecurityWarning(context.Background(), cfg, func(_ context.Context, ev events.Event) {
		emitted = append(emitted, ev)
	})
	if len(emitted) != 0 {
		t.Fatalf("expected no-op warning emitter, got %d events", len(emitted))
	}
}

func TestEmitCodeExecProvisioningSecurityWarning_SkipsWhenDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	emitted := 0
	emitCodeExecProvisioningSecurityWarning(context.Background(), cfg, func(_ context.Context, _ events.Event) {
		emitted++
	})
	if emitted != 0 {
		t.Fatalf("expected no warning events when auto_provision=false")
	}
}
