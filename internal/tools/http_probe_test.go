package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func runProbe(t *testing.T, p *HTTPProbe, in httpProbeInput) (string, error) {
	t.Helper()
	raw, _ := json.Marshal(in)
	return p.Run(context.Background(), raw)
}

func TestHTTPProbeReportsStatusHeadersAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"db unavailable"}`))
	}))
	defer srv.Close()

	got, err := runProbe(t, NewHTTPProbe(), httpProbeInput{URL: srv.URL + "/login"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET " + srv.URL + "/login -> 500 Internal Server Error", "Content-Type: application/json", `{"error":"db unavailable"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestHTTPProbeHeadHasNoBody(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Write([]byte("body"))
	}))
	defer srv.Close()

	got, err := runProbe(t, NewHTTPProbe(), httpProbeInput{URL: srv.URL, Method: "head"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodHead || !strings.Contains(got, "HEAD ") || strings.Contains(got, "body") {
		t.Fatalf("method=%s output=%q", gotMethod, got)
	}
}

func TestHTTPProbeTruncatesLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", 5000)))
	}))
	defer srv.Close()

	got, _ := runProbe(t, NewHTTPProbe(), httpProbeInput{URL: srv.URL})
	_, afterHeaders, _ := strings.Cut(got, "\n\n")
	body, note, found := strings.Cut(afterHeaders, "\n[body truncated]")
	if !found || note != "" || strings.Count(body, "a") != maxProbeBody {
		t.Fatalf("unexpected truncation: found=%v, %d a's", found, strings.Count(body, "a"))
	}
}

func TestHTTPProbeConnectionFailureIsAFinding(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening any more

	got, err := runProbe(t, NewHTTPProbe(), httpProbeInput{URL: url})
	if err != nil {
		t.Fatalf("connection failure should not be a tool error: %v", err)
	}
	if !strings.HasPrefix(got, "request failed") {
		t.Fatalf("got %q", got)
	}
}

func TestHTTPProbeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	p := &HTTPProbe{client: &http.Client{Timeout: 50 * time.Millisecond}}
	got, err := runProbe(t, p, httpProbeInput{URL: srv.URL})
	if err != nil || !strings.HasPrefix(got, "request failed") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestHTTPProbeRejectsBadInput(t *testing.T) {
	p := NewHTTPProbe()
	for _, in := range []httpProbeInput{
		{URL: ""},
		{URL: "file:///etc/passwd"},
		{URL: "localhost:8080"},
		{URL: "http://example.com", Method: "DELETE"},
	} {
		if _, err := runProbe(t, p, in); err == nil {
			t.Errorf("expected error for %+v", in)
		}
	}
}
