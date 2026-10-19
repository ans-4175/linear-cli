package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientVariablesAndAuthorization(t *testing.T) {
	text := "Hello \"quotes\" \\ Unicode 日本語\n\nend\n"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "test-token" {
			t.Error("bad method/auth")
		}
		var body struct {
			Query     string
			Variables map[string]any
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body.Variables["text"] != text {
			t.Errorf("text corrupted: %q", body.Variables["text"])
		}
		if strings.Contains(body.Query, text) {
			t.Error("content interpolated into source")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer s.Close()
	c := newAPIClient(s.URL, "test-token", time.Second)
	out, e := c.do(context.Background(), "mutation X($text: String!) { x(text: $text) }", object{"text": text})
	if e != nil || out["ok"] != true {
		t.Fatal(out, e)
	}
}
func TestClientErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"graphql-200", 200, `{"data":{"partial":true},"errors":[{"message":"invalid token test-secret","extensions":{"code":"FORBIDDEN"}}]}`, "[REDACTED]"},
		{"http-401", 401, `{"errors":[{"message":"unauthorized"}]}`, "HTTP 401"},
		{"rate-limit", 400, `{"errors":[{"message":"slow down","extensions":{"code":"RATELIMITED"}}]}`, "RATELIMITED"},
		{"http-html", 502, "<html>bad gateway</html>", "HTTP 502"},
		{"invalid-json", 200, "not json", "invalid JSON"},
		{"no-data", 200, `{}`, "no data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, e := newAPIClient(s.URL, "test-secret", time.Second).do(context.Background(), "query { viewer { id } }", nil)
			if e == nil || !strings.Contains(e.Error(), tc.want) || strings.Contains(e.Error(), "test-secret") {
				t.Fatal(e)
			}
			if calls.Load() != 1 {
				t.Fatal("request retried")
			}
			if tc.name == "graphql-200" {
				var ae *apiError
				if !errors.As(e, &ae) || ae.Data["partial"] != true {
					t.Fatal("partial data not preserved", e)
				}
			}
		})
	}
}
func TestClientDoesNotFollowRedirects(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer source.Close()
	_, e := newAPIClient(source.URL, "secret", time.Second).do(context.Background(), "query { viewer { id } }", nil)
	if e == nil || leaked.Load() {
		t.Fatal("followed redirect", e)
	}
}
func TestClientCancellationAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newAPIClient("http://127.0.0.1:1", "secret", time.Second)
	if _, e := c.do(ctx, "query { viewer { id } }", nil); e == nil {
		t.Fatal("cancel ignored")
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		w.Write([]byte(`{"data":{}}`))
	}))
	defer s.Close()
	if _, e := newAPIClient(s.URL, "secret", time.Millisecond).do(context.Background(), "query { viewer { id } }", nil); e == nil {
		t.Fatal("timeout ignored")
	}
}
