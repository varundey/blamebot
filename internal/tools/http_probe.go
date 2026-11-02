package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	probeTimeout  = 10 * time.Second
	maxProbeBody  = 2000
	probeBodyRead = maxProbeBody + 1
)

// HTTPProbe makes a single GET or HEAD request, for checking whether an
// endpoint is up and how it answers.
type HTTPProbe struct {
	client *http.Client
}

type httpProbeInput struct {
	URL    string `json:"url"`
	Method string `json:"method"`
}

// NewHTTPProbe returns an HTTPProbe with a short timeout.
func NewHTTPProbe() *HTTPProbe {
	return &HTTPProbe{client: &http.Client{Timeout: probeTimeout}}
}

func (*HTTPProbe) Name() string { return "http_probe" }

func (*HTTPProbe) Description() string {
	return "Send one GET or HEAD request to an http(s) URL and report the status, latency, key response headers, " +
		"and the first 2000 bytes of the body. Use it to check whether an endpoint is healthy and what error it returns."
}

func (*HTTPProbe) Schema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"url":    map[string]any{"type": "string", "description": "Full http:// or https:// URL."},
			"method": map[string]any{"type": "string", "enum": []string{"GET", "HEAD"}, "description": "HTTP method (default GET)."},
		},
		Required: []string{"url"},
	}
}

func (p *HTTPProbe) Run(ctx context.Context, raw json.RawMessage) (string, error) {
	var in httpProbeInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("url must be an absolute http(s) URL, got %q", in.URL)
	}
	method := strings.ToUpper(in.Method)
	switch method {
	case "":
		method = http.MethodGet
	case http.MethodGet, http.MethodHead:
	default:
		return "", fmt.Errorf("method must be GET or HEAD, got %q", in.Method)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return "", err
	}
	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		// Connection refused, DNS failure, and timeouts are findings, not tool failures.
		return fmt.Sprintf("request failed after %s: %v", time.Since(start).Round(time.Millisecond), err), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, probeBodyRead))
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	elapsed := time.Since(start)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s -> %s in %s\n", method, u.String(), resp.Status, elapsed.Round(time.Millisecond))
	for _, h := range []string{"Content-Type", "Content-Length", "Server", "Retry-After", "Location", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			fmt.Fprintf(&sb, "%s: %s\n", h, v)
		}
	}
	if method == http.MethodGet && len(body) > 0 {
		text := string(body)
		suffix := ""
		if len(body) > maxProbeBody {
			text, suffix = string(body[:maxProbeBody]), "\n[body truncated]"
		}
		fmt.Fprintf(&sb, "\n%s%s", text, suffix)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
