package rpc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	locationapp "github.com/tinoosan/agen8-mcp-server/internal/services/location/app"
	locationdomain "github.com/tinoosan/agen8-mcp-server/internal/services/location/domain"
	locationinfra "github.com/tinoosan/agen8-mcp-server/internal/services/location/infra"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func TestRegisterLocationListAndListDir(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := newLocationRPCServerForTest(t)

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"location.list",
		"params":{}
	}`))
	if err != nil {
		t.Fatalf("Handle location.list: %v", err)
	}
	var listResp struct {
		Result struct {
			Locations []struct {
				ID    string `json:"id"`
				Ready bool   `json:"ready"`
			} `json:"locations"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(raw, &listResp); err != nil {
		t.Fatalf("unmarshal location.list: %v", err)
	}
	if listResp.Error != nil {
		t.Fatalf("location.list error = %+v", listResp.Error)
	}
	if len(listResp.Result.Locations) != 1 || listResp.Result.Locations[0].ID != "local" || !listResp.Result.Locations[0].Ready {
		t.Fatalf("location.list result = %+v", listResp.Result.Locations)
	}

	tmp := t.TempDir()
	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc":"2.0",
		"id":2,
		"method":"location.fs.listDir",
		"params":{"locationId":"local","path":`+quoteJSON(tmp)+`}
	}`))
	if err != nil {
		t.Fatalf("Handle location.fs.listDir: %v", err)
	}
	var dirResp struct {
		Result struct {
			Entries []struct {
				Path string `json:"path"`
			} `json:"entries"`
		} `json:"result"`
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(raw, &dirResp); err != nil {
		t.Fatalf("unmarshal location.fs.listDir: %v", err)
	}
	if dirResp.Error != nil {
		t.Fatalf("location.fs.listDir error = %+v", dirResp.Error)
	}
}

func TestRegisterLocationListDirRequiresPath(t *testing.T) {
	t.Parallel()
	server := newLocationRPCServerForTest(t)
	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"location.fs.listDir",
		"params":{"locationId":"local"}
	}`))
	if err != nil {
		t.Fatalf("Handle location.fs.listDir: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error = %+v", resp.Error)
	}
}

func TestRegisterLocationInstallCodexRequiresLocationID(t *testing.T) {
	t.Parallel()
	server := newLocationRPCServerForTest(t)
	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc":"2.0",
		"id":1,
		"method":"location.installCodex",
		"params":{}
	}`))
	if err != nil {
		t.Fatalf("Handle location.installCodex: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error = %+v", resp.Error)
	}
}

func newLocationRPCServerForTest(t *testing.T) *Server {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repo, err := locationinfra.NewRepository(handle)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	svc, err := locationapp.NewService(locationapp.Config{
		Locations: repo,
		Transport: locationinfra.NewTransport(),
		Projects:  noProjectRefs{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.EnsureLocal(context.Background()); err != nil {
		t.Fatalf("EnsureLocal: %v", err)
	}
	reg := NewRegistry()
	if err := RegisterLocation(reg, svc); err != nil {
		t.Fatalf("RegisterLocation: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server
}

type noProjectRefs struct{}

func (noProjectRefs) HasProjectsForLocation(context.Context, locationdomain.ID) (bool, error) {
	return false, nil
}

func quoteJSON(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
