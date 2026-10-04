package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDevWebProxyIntegrationPreservesRoutingAndStripsCredentials(t *testing.T) {
	type received struct {
		Path, Query, Host, Authorization, Cookie, Forwarded, ForwardedFor string
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(received{r.URL.Path, r.URL.RawQuery, r.Host,
			r.Header.Get("Authorization"), r.Header.Get("Cookie"), r.Header.Get("Forwarded"), r.Header.Get("X-Forwarded-For")}); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	t.Setenv(EnvDevWebURL, upstream.URL+"/base?origin=dev")
	handler, err := (&Daemon{}).webHandler()
	if err != nil {
		t.Fatal(err)
	}
	frontend := httptest.NewServer(handler)
	defer frontend.Close()
	req, err := http.NewRequest(http.MethodGet, frontend.URL+"/assets/work.js?version=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "localhost:8080"
	req.Header.Set("Authorization", "Bearer synthetic-credential")
	req.Header.Set("Cookie", "synthetic=session")
	req.Header.Set("Forwarded", "for=203.0.113.8")
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	resp, err := frontend.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got received
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || got.Path != "/base/assets/work.js" || got.Query != "origin=dev&version=1" || got.Host != req.Host {
		t.Fatalf("proxy routing: status=%d received=%+v", resp.StatusCode, got)
	}
	if got.Authorization != "" || got.Cookie != "" || got.Forwarded != "" || got.ForwardedFor != "127.0.0.1" {
		t.Fatalf("proxy forwarded credentials or untrusted client headers: %+v", got)
	}
}
