package daemon

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	httptool "github.com/tinoosan/agen8/internal/mcp/tools/http"
	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
)

type httpCredentialResolver struct {
	credentials *credentialapp.Service
	userID      string
	projectID   string
}

func (r httpCredentialResolver) ResolveHTTP(ctx context.Context, host string) (httptool.HTTPCredential, bool, error) {
	if r.credentials == nil {
		return httptool.HTTPCredential{}, false, nil
	}
	ctx = caller.ContextWithCaller(ctx, caller.Caller{
		UserID:    strings.TrimSpace(r.userID),
		ProjectID: types.ProjectID(strings.TrimSpace(r.projectID)),
	})
	host = normalizeCredentialHost(host)
	if host == "" {
		return httptool.HTTPCredential{}, false, nil
	}
	records, err := r.credentials.ListCredentials(ctx, credentialdomain.Filter{
		Kind:   credentialdomain.KindAPIKey,
		Status: credentialdomain.StatusActive,
	})
	if err != nil {
		return httptool.HTTPCredential{}, false, fmt.Errorf("http credential lookup: %w", err)
	}

	var single *httptool.HTTPCredential
	headers := map[string]string{}
	for _, record := range records {
		resolved, err := r.credentials.ResolveCredential(ctx, credentialapp.ResolveCredentialInput{
			CredentialID: record.ID(),
			Purpose:      credentialdomain.PurposeHTTPTool,
		})
		if err != nil {
			return httptool.HTTPCredential{}, false, fmt.Errorf("resolve http credential %s: %w", record.ID(), err)
		}
		values := cleanCredentialValues(resolved.Values)
		if normalizeCredentialHost(values["host"]) != host {
			continue
		}
		injection := credentialdomain.InjectionMode(strings.ToLower(strings.TrimSpace(values["injection"])))
		if injection == "" {
			injection = credentialdomain.InjectionBearer
		}
		fieldName := canonicalHTTPHeaderName(httpCredentialFieldName(injection, values))
		candidate := httptool.HTTPCredential{
			CredentialID: record.ID(),
			Injection:    injection,
			FieldName:    fieldName,
			Values:       values,
		}
		if injection == credentialdomain.InjectionHeader {
			value := httpCredentialValue(values)
			if fieldName == "" || value == "" {
				return httptool.HTTPCredential{}, false, fmt.Errorf("http credential %s requires headerName and value", record.ID())
			}
			if _, exists := headers[fieldName]; exists {
				return httptool.HTTPCredential{}, false, fmt.Errorf("duplicate http credential header %q for host %s", fieldName, host)
			}
			headers[fieldName] = value
			continue
		}
		if single != nil {
			return httptool.HTTPCredential{}, false, fmt.Errorf("multiple non-header http credentials match host %s", host)
		}
		clone := candidate
		single = &clone
	}
	if len(headers) > 0 {
		if single != nil {
			return httptool.HTTPCredential{}, false, fmt.Errorf("mixed header and non-header http credentials match host %s", host)
		}
		return httptool.HTTPCredential{
			Injection: credentialdomain.InjectionHeader,
			Headers:   headers,
		}, true, nil
	}
	if single != nil {
		return *single, true, nil
	}
	return httptool.HTTPCredential{}, false, nil
}

func normalizeCredentialHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if !strings.Contains(host, "://") && strings.Contains(host, "/") {
		host = "https://" + host
	}
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	if strings.Contains(host, ":") {
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
	}
	return strings.Trim(host, "[]")
}

func cleanCredentialValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func httpCredentialFieldName(injection credentialdomain.InjectionMode, values map[string]string) string {
	switch injection {
	case credentialdomain.InjectionHeader:
		return firstCredentialValue(values, "headerName", "header", "name")
	case credentialdomain.InjectionQuery:
		return firstCredentialValue(values, "paramName", "param", "name")
	default:
		return ""
	}
}

func httpCredentialValue(values map[string]string) string {
	return firstCredentialValue(values, "value", "token", "api_key", "apikey", "key", "secret")
}

func firstCredentialValue(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values[name]); value != "" {
			return value
		}
	}
	return ""
}

func canonicalHTTPHeaderName(name string) string {
	name = strings.TrimSpace(name)
	switch strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(name)) {
	case "ALPACA_PAPER_API_KEY", "ALPACA_API_KEY", "ALPACA_API_KEY_ID", "APCA_API_KEY_ID":
		return "APCA-API-KEY-ID"
	case "ALPACA_PAPER_SECRET_KEY", "ALPACA_SECRET_KEY", "ALPACA_API_SECRET_KEY", "APCA_API_SECRET_KEY", "APCA_PAPER_SECRET_KEY":
		return "APCA-API-SECRET-KEY"
	default:
		return name
	}
}
