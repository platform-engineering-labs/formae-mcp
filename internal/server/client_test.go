package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestFormaeClient creates a FormaeClient pointed at the given httptest.Server.
func newTestFormaeClient(srv *httptest.Server) *FormaeClient {
	c := NewFormaeClient(srv.URL)
	c.httpClient = srv.Client()
	return c
}

// TestSubmitCommandAccepts202 verifies that the happy path (async-accepted) still works.
func TestSubmitCommandAccepts202(t *testing.T) {
	const respBody = `{"CommandId":"abc123"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/commands" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	body, err := c.SubmitCommand("apply", "reconcile", false, false, nil, "client-1")
	if err != nil {
		t.Fatalf("SubmitCommand: unexpected error: %v", err)
	}
	if string(body) != respBody {
		t.Fatalf("want body %q, got %q", respBody, string(body))
	}
}

// TestSubmitCommandAccepts200Simulate verifies that a synchronous simulate response
// (200 OK) is treated as success, not an error.
func TestSubmitCommandAccepts200Simulate(t *testing.T) {
	const respBody = `{"ChangesRequired":true,"Plan":[{"action":"create"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/commands" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	body, err := c.SubmitCommand("apply", "reconcile", true, false, nil, "client-1")
	if err != nil {
		t.Fatalf("SubmitCommand with simulate=true and 200: unexpected error: %v", err)
	}
	if string(body) != respBody {
		t.Fatalf("want body %q, got %q", respBody, string(body))
	}
}

// TestSubmitCommandRejectsOtherStatus verifies that non-200/202 statuses remain errors.
func TestSubmitCommandRejectsOtherStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	_, err := c.SubmitCommand("apply", "reconcile", false, false, nil, "client-1")
	if err == nil {
		t.Fatal("SubmitCommand: expected error for 500, got nil")
	}
}

// TestDestroyByQueryAccepts202 verifies that the async-accepted path still works.
func TestDestroyByQueryAccepts202(t *testing.T) {
	const respBody = `{"CommandId":"xyz789"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/commands" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	body, err := c.DestroyByQuery("stack=default", false, "client-1")
	if err != nil {
		t.Fatalf("DestroyByQuery: unexpected error: %v", err)
	}
	if string(body) != respBody {
		t.Fatalf("want body %q, got %q", respBody, string(body))
	}
}

// TestDestroyByQueryAccepts200Simulate verifies that a synchronous simulate destroy
// response (200 OK) is treated as success, not an error.
func TestDestroyByQueryAccepts200Simulate(t *testing.T) {
	const respBody = `{"ChangesRequired":true,"Plan":[{"action":"delete"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/commands" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respBody))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	body, err := c.DestroyByQuery("stack=default", true, "client-1")
	if err != nil {
		t.Fatalf("DestroyByQuery with simulate=true and 200: unexpected error: %v", err)
	}
	if string(body) != respBody {
		t.Fatalf("want body %q, got %q", respBody, string(body))
	}
}

// TestDestroyByQueryRejectsOtherStatus verifies that non-200/202 statuses remain errors.
func TestDestroyByQueryRejectsOtherStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	c := newTestFormaeClient(srv)
	_, err := c.DestroyByQuery("stack=default", false, "client-1")
	if err == nil {
		t.Fatal("DestroyByQuery: expected error for 500, got nil")
	}
}
