// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPingAndCRUD(t *testing.T) {
	t.Parallel()

	store := map[string]json.RawMessage{
		"/config/": json.RawMessage(`{"apps":{"http":{"servers":{}}}}`),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Etag", `"/config 1"`)
		switch r.Method {
		case http.MethodGet:
			body, ok := store[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case http.MethodPut, http.MethodPost, http.MethodPatch:
			payload, _ := io.ReadAll(r.Body)
			store[r.URL.Path] = payload
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(store, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	if err := c.Put(ctx, "/config/apps/http/servers/srv", map[string]any{
		"@id":    "srv",
		"listen": []string{":80"},
	}); err != nil {
		t.Fatal(err)
	}

	raw, _, err := c.Get(ctx, "/config/apps/http/servers/srv")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("expected body")
	}

	if err := c.Delete(ctx, "/config/apps/http/servers/srv"); err != nil {
		t.Fatal(err)
	}
}

func TestUnixSocket(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	socket := filepath.Join(dir, "admin.sock")

	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = l.Close()
		_ = os.Remove(socket)
	})

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}),
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	c, err := New("unix://"+socket, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	err := &APIError{Status: 404, Path: "/id/x", Body: "nil"}
	if !IsNotFound(err) {
		t.Fatal("expected not found")
	}
	if IsNotFound(nil) {
		t.Fatal("nil is not not-found")
	}
}

func TestTransportFor(t *testing.T) {
	t.Parallel()
	base, _, err := transportFor("unix:///run/caddy.sock")
	if err != nil {
		t.Fatal(err)
	}
	if base != "http://localhost" {
		t.Fatalf("got %s", base)
	}
	base, _, err = transportFor("http://127.0.0.1:2019/")
	if err != nil {
		t.Fatal(err)
	}
	if base != "http://127.0.0.1:2019" {
		t.Fatalf("got %s", base)
	}
}
