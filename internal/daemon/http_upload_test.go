package daemon

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/config"
)

func TestHandleFileUploadAcceptsFiveMegabytesAndKeepsRPCCap(t *testing.T) {
	projectRoot := t.TempDir()
	handler := newUploadTestHandler(t)
	sessionToken := setupSessionForEventsTest(t, handler)
	projectID := createProjectForLinkTokenTest(t, handler, sessionToken, projectRoot)

	body, contentType := multipartUploadBody(t, map[string]string{
		"projectId": projectID,
		"path":      "/project/.agen8/attachments/task-1/five.bin",
	}, bytes.Repeat([]byte{0x5a}, 5<<20), "five.bin")
	req := httptest.NewRequest(http.MethodPost, "/uploads/files", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if result.Path != "/project/.agen8/attachments/task-1/five.bin" {
		t.Fatalf("path=%q", result.Path)
	}
	info, err := os.Stat(filepath.Join(projectRoot, ".agen8", "attachments", "task-1", "five.bin"))
	if err != nil {
		t.Fatalf("stat uploaded file: %v", err)
	}
	if info.Size() != 5<<20 {
		t.Fatalf("uploaded size=%d want %d", info.Size(), 5<<20)
	}

	rpcPayload := `{"jsonrpc":"2.0","id":1,"method":"auth.login","params":{"username":"` + strings.Repeat("a", maxRPCRequestBodyBytes) + `"}}`
	rpcReq := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(rpcPayload))
	rpcReq.ContentLength = int64(maxRPCRequestBodyBytes + 1)
	rpcRec := httptest.NewRecorder()
	handler.ServeHTTP(rpcRec, rpcReq)
	if rpcRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("rpc status=%d want %d body=%s", rpcRec.Code, http.StatusRequestEntityTooLarge, rpcRec.Body.String())
	}
}

func TestHandleFileUploadRejectsOverCeilingCleanly(t *testing.T) {
	projectRoot := t.TempDir()
	handler := newUploadTestHandler(t)
	sessionToken := setupSessionForEventsTest(t, handler)
	projectID := createProjectForLinkTokenTest(t, handler, sessionToken, projectRoot)

	body, contentType := multipartUploadBody(t, map[string]string{
		"projectId": projectID,
		"path":      "/project/.agen8/attachments/task-1/too-big.bin",
	}, bytes.Repeat([]byte{0x78}, int(maxAttachmentUploadFileBytes+1)), "too-big.bin")
	req := httptest.NewRequest(http.MethodPost, "/uploads/files", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "attachment upload file is too large") {
		t.Fatalf("body=%q want clean size-limit error", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".agen8", "attachments", "task-1", "too-big.bin")); !os.IsNotExist(err) {
		t.Fatalf("oversized upload wrote a file, stat err=%v", err)
	}
}

func newUploadTestHandler(t *testing.T) http.Handler {
	t.Helper()
	d, err := New(Config{
		AppConfig:  config.Config{DataDir: t.TempDir()},
		SetupToken: "test-setup-token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	handler, err := d.httpHandler()
	if err != nil {
		t.Fatalf("httpHandler: %v", err)
	}
	return handler
}

func multipartUploadBody(t *testing.T, fields map[string]string, data []byte, fileName string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
