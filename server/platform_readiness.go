package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	schedulerSecurePort         = 10259
	controllerManagerSecurePort = 10257
)

var controlPlaneReady atomic.Bool

// PlatformReady reports whether the embedded scheduler and controller-manager
// have passed their own readiness endpoints. The apiserver alone is not enough
// to safely accept workload installations.
func PlatformReady() bool {
	return controlPlaneReady.Load()
}

// WaitForControlPlaneReady blocks until both embedded control-plane workers are
// serving readiness responses or the process context is cancelled.
func WaitForControlPlaneReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	client := platformReadinessClient()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := checkControlPlaneReady(ctx, client); err == nil {
			controlPlaneReady.Store(true)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// StartPlatformReadinessMonitor keeps the gate honest after startup. A
// component crash makes new Helm operations fail fast until it recovers.
func StartPlatformReadinessMonitor(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		client := platformReadinessClient()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			controlPlaneReady.Store(checkControlPlaneReady(ctx, client) == nil)
			select {
			case <-ctx.Done():
				controlPlaneReady.Store(false)
				return
			case <-ticker.C:
			}
		}
	}()
}

func platformReadinessClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // #nosec G402: local component health endpoints use self-signed certificates.
		Timeout:   2 * time.Second,
	}
}

func checkControlPlaneReady(ctx context.Context, client *http.Client) error {
	for _, port := range []int{schedulerSecurePort, controllerManagerSecurePort} {
		if err := checkComponentReady(ctx, client, port); err != nil {
			return err
		}
	}
	return nil
}

func checkComponentReady(ctx context.Context, client *http.Client, port int) error {
	url := "https://127.0.0.1:" + strconv.Itoa(port) + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("component %d readiness: %w", port, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("component %d readiness returned %s", port, resp.Status)
	}
	return nil
}
