// Copyright 2026 The Casos Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForServerReturnsOnceHealthy(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("polled %q, want /api/health", r.URL.Path)
		}

		// Unhealthy for the first few polls, the way a server that is still
		// binding its listener would be.
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","msg":"","data":"casos","data2":null}`))
	}))
	defer server.Close()

	if err := WaitForServer(server.URL, 5*time.Second); err != nil {
		t.Fatalf("WaitForServer: %v", err)
	}
	if got := attempts.Load(); got < 3 {
		t.Errorf("served %d requests, want at least 3", got)
	}
}

func TestWaitForServerAcceptsTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("polled %q, want /api/health", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":"ok","data":"casos"}`))
	}))
	defer server.Close()

	if err := WaitForServer(server.URL+"/", 5*time.Second); err != nil {
		t.Fatalf("WaitForServer: %v", err)
	}
}

// A different program holding the port is the usual reason CasOS fails to
// start, so answering the poll is not on its own enough to count as ready.
func TestWaitForServerRejectsAnotherService(t *testing.T) {
	tests := map[string]string{
		"foreign json": `{"status":"ok","data":"something-else"}`,
		"error status": `{"status":"error","msg":"nope","data":"casos"}`,
		"not json":     `<html><title>CasOS</title></html>`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()

			if err := WaitForServer(server.URL, 500*time.Millisecond); err == nil {
				t.Fatal("WaitForServer succeeded, want timeout")
			}
		})
	}
}

func TestWaitForServerTimesOutWhenNothingListens(t *testing.T) {
	// Bound and immediately closed, so the port is free and connections are
	// refused rather than left hanging.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	start := time.Now()
	err := WaitForServer(url, 500*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForServer succeeded, want timeout")
	}
	if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
		t.Errorf("returned after %s, want at least the 500ms timeout", elapsed)
	}
}
