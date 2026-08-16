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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/inconshreveable/mousetrap"
)

const (
	readyPollInterval   = 200 * time.Millisecond
	readyRequestTimeout = 2 * time.Second
	readyBodyLimit      = 4 << 10
	readyMarker         = "casos"
)

// healthResponse is the subset of controllers.Response that /api/health fills
// in. It is redeclared here rather than imported so util stays a leaf package.
type healthResponse struct {
	Status string `json:"status"`
	Data   string `json:"data"`
}

// StartedByDoubleClick reports whether the process was launched from Explorer —
// a double-clicked casos.exe, a desktop shortcut or a Start menu entry — rather
// than from a terminal or a service manager. It is always false off Windows.
//
// This calls the same detector cobra consults for its mousetrap check, so the
// two can never disagree about what kind of launch this is.
func StartedByDoubleClick() bool {
	return mousetrap.StartedByExplorer()
}

// WaitForServer blocks until baseURL serves the CasOS health endpoint, or until
// timeout elapses. Checking for the CasOS marker rather than any HTTP 200 keeps
// an unrelated process that already holds the port — the usual reason CasOS
// fails to start — from being mistaken for a CasOS that came up fine.
func WaitForServer(baseURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	healthURL := strings.TrimSuffix(baseURL, "/") + "/api/health"
	client := &http.Client{Timeout: readyRequestTimeout}
	ticker := time.NewTicker(readyPollInterval)
	defer ticker.Stop()

	for {
		if serverReady(ctx, client, healthURL) {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s waiting for CasOS at %s", timeout, baseURL)
		case <-ticker.C:
		}
	}
}

func serverReady(ctx context.Context, client *http.Client, healthURL string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}

	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, readyBodyLimit))
	if err != nil {
		return false
	}

	var health healthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return false
	}

	return health.Status == "ok" && health.Data == readyMarker
}
