package protocol

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseEndpoint(t *testing.T) {
	tcp := ParseEndpoint("127.0.0.1:7777")
	if tcp.Scheme != "tcp" || tcp.Target != "127.0.0.1:7777" {
		t.Fatalf("unexpected tcp endpoint: %+v", tcp)
	}

	unix := ParseEndpoint("unix:///tmp/agen8.sock")
	if unix.Scheme != "unix" || unix.Target != "/tmp/agen8.sock" {
		t.Fatalf("unexpected unix endpoint: %+v", unix)
	}
}

func TestDefaultRPCEndpoint(t *testing.T) {
	got := DefaultRPCEndpoint()
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(got, "npipe://") {
			t.Fatalf("windows default = %q", got)
		}
		return
	}
	if !strings.HasPrefix(got, "unix://") {
		t.Fatalf("non-windows default = %q", got)
	}
	if !strings.HasSuffix(got, filepath.Join(".agen8", "daemon.sock")) {
		t.Fatalf("unexpected socket path: %q", got)
	}
}
