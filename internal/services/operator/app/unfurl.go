package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type UnfurlResult struct {
	Title   string
	Favicon string
}

var (
	unfurlHTTPClient = &http.Client{Timeout: 3 * time.Second}
	titleTagPattern  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

func Unfurl(ctx context.Context, rawURL string) UnfurlResult {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return UnfurlResult{}
	}
	fallback := parsed.Hostname()
	if strings.TrimSpace(parsed.Scheme) == "" {
		return UnfurlResult{Title: fallback}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return UnfurlResult{Title: fallback}
	}
	resp, err := unfurlHTTPClient.Do(req)
	if err != nil {
		return UnfurlResult{Title: fallback}
	}
	defer func() { _ = resp.Body.Close() }()

	result := UnfurlResult{Title: fallback}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(contentType, "text/html") {
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return result
	}
	matches := titleTagPattern.FindSubmatch(body)
	if len(matches) < 2 {
		return result
	}
	title := strings.TrimSpace(htmlTitleText(string(matches[1])))
	if title == "" {
		return result
	}
	result.Title = title
	result.Favicon = fmt.Sprintf("%s://%s/favicon.ico", parsed.Scheme, parsed.Host)
	return result
}

func htmlTitleText(value string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
	)
	return replacer.Replace(strings.Join(strings.Fields(value), " "))
}
