package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/rpc"
	fileapp "github.com/tinoosan/agen8/internal/services/file/app"
)

const (
	maxAttachmentUploadFileBytes    int64 = 25 << 20
	maxAttachmentUploadRequestBytes int64 = maxAttachmentUploadFileBytes + (1 << 20)
	maxAttachmentUploadFieldBytes   int64 = 64 << 10
)

var errAttachmentUploadFileTooLarge = errors.New("attachment upload file is too large")

func (d *Daemon) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	usesSessionCookie := strings.TrimSpace(r.Header.Get("Authorization")) == "" && requestHasSessionCookie(r)
	if usesSessionCookie && !checkSameOriginRequest(r) {
		http.Error(w, "cross-origin request blocked", http.StatusForbidden)
		return
	}
	identity, err := d.httpIdentityFromRequest(r.Context(), r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := rpc.ContextWithIdentity(r.Context(), identity)
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:   identity.UserID,
		MemberID: identity.MemberID,
		Role:     identity.Role,
	})

	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentUploadRequestBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		status := http.StatusBadRequest
		message := "multipart upload is required"
		if uploadRequestTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
			message = "attachment upload request is too large"
		}
		http.Error(w, message, status)
		return
	}

	input, err := decodeFileUploadMultipart(reader)
	if err != nil {
		status := http.StatusBadRequest
		message := err.Error()
		if errors.Is(err, errAttachmentUploadFileTooLarge) || uploadRequestTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
			message = "attachment upload file is too large"
		}
		http.Error(w, message, status)
		return
	}
	result, err := d.app.FileSvc.UploadReader(ctx, input)
	if err != nil {
		if errors.Is(err, errAttachmentUploadFileTooLarge) || uploadRequestTooLarge(err) {
			http.Error(w, "attachment upload file is too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "attachment upload failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func decodeFileUploadMultipart(reader *multipart.Reader) (fileapp.UploadReaderInput, error) {
	var input fileapp.UploadReaderInput
	var fileSeen bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fileapp.UploadReaderInput{}, err
		}
		switch part.FormName() {
		case "projectId":
			value, err := readUploadField(part)
			if err != nil {
				return fileapp.UploadReaderInput{}, fmt.Errorf("projectId: %w", err)
			}
			input.ProjectID = types.ProjectID(value)
		case "projectRoot":
			value, err := readUploadField(part)
			if err != nil {
				return fileapp.UploadReaderInput{}, fmt.Errorf("projectRoot: %w", err)
			}
			input.ProjectRoot = value
		case "path":
			value, err := readUploadField(part)
			if err != nil {
				return fileapp.UploadReaderInput{}, fmt.Errorf("path: %w", err)
			}
			input.Path = value
		case "file":
			if fileSeen {
				return fileapp.UploadReaderInput{}, fmt.Errorf("file field must be provided once")
			}
			input.Reader = &attachmentUploadLimitReader{r: part, remaining: maxAttachmentUploadFileBytes}
			fileSeen = true
			return input, validateFileUploadInput(input, fileSeen)
		}
	}
	return input, validateFileUploadInput(input, fileSeen)
}

func validateFileUploadInput(input fileapp.UploadReaderInput, fileSeen bool) error {
	if strings.TrimSpace(string(input.ProjectID)) == "" && strings.TrimSpace(input.ProjectRoot) == "" {
		return fmt.Errorf("projectId is required")
	}
	if strings.TrimSpace(input.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if !fileSeen {
		return fmt.Errorf("file is required")
	}
	return nil
}

func readUploadField(part *multipart.Part) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxAttachmentUploadFieldBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxAttachmentUploadFieldBytes {
		return "", fmt.Errorf("field is too large")
	}
	return strings.TrimSpace(string(data)), nil
}

type attachmentUploadLimitReader struct {
	r         io.Reader
	remaining int64
}

func (r *attachmentUploadLimitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		if len(p) == 0 {
			return 0, nil
		}
		one := p[:1]
		n, err := r.r.Read(one)
		if n > 0 {
			return 0, errAttachmentUploadFileTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func uploadRequestTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "request body too large")
}
