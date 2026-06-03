package infra

import (
	"testing"
)

// TestValidateWebhookURL verifies that SSRF-prone URLs are rejected. (P1-4)
func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com/webhook", false},
		{"http://example.com/webhook", false},
		{"http://localhost/internal", true},
		{"http://127.0.0.1/metadata", true},
		{"http://169.254.169.254/latest/meta-data", true},
		{"http://[::1]/path", true},
		{"http://10.0.0.1/path", true},
		{"http://192.168.1.1/path", true},
		{"http://172.16.0.1/path", true},
		{"ftp://example.com/path", true},
		{"file:///etc/passwd", true},
		{"", true},
		{"not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
