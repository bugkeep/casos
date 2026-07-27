package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	proxypkg "github.com/casosorg/casos/proxy"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

const (
	helmDownloadAttemptTimeout = 2 * time.Minute
	helmDownloadCacheTTL       = 15 * time.Minute
	helmDownloadCacheEntries   = 32
	helmDownloadCacheBytes     = 128 << 20
)

var defaultHelmArtifactCache = newHelmArtifactCache(helmDownloadCacheTTL, helmDownloadCacheEntries, helmDownloadCacheBytes)

func downloadHelmArtifact(ctx context.Context, rawURL string) ([]byte, error) {
	downloader := newHelmArtifactDownloader(
		proxypkg.ProxyHttpClient,
		helmDownloadAttemptTimeout,
		[]time.Duration{time.Second, 3 * time.Second},
		defaultHelmArtifactCache,
	)
	return downloader.download(ctx, rawURL)
}

type helmHTTPStatusError struct {
	statusCode int
}

func (e *helmHTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP %d", e.statusCode)
}

type helmArtifactDownloader struct {
	client         *http.Client
	attemptTimeout time.Duration
	retryDelays    []time.Duration
	cache          *helmArtifactCache
}

func newHelmArtifactDownloader(client *http.Client, attemptTimeout time.Duration, retryDelays []time.Duration, cache *helmArtifactCache) *helmArtifactDownloader {
	if client == nil {
		client = http.DefaultClient
	}
	return &helmArtifactDownloader{
		client:         client,
		attemptTimeout: attemptTimeout,
		retryDelays:    append([]time.Duration(nil), retryDelays...),
		cache:          cache,
	}
}

func (d *helmArtifactDownloader) download(ctx context.Context, rawURL string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if d.cache != nil {
		if data, ok := d.cache.get(rawURL); ok {
			return data, nil
		}
	}

	for attempt := 0; ; attempt++ {
		data, err := d.downloadOnce(ctx, rawURL)
		if err == nil {
			if d.cache != nil {
				d.cache.put(rawURL, data)
			}
			return data, nil
		}
		if ctx.Err() != nil || attempt >= len(d.retryDelays) || !isRetryableHelmDownloadError(err) {
			return nil, err
		}
		if err := waitForHelmDownloadRetry(ctx, d.retryDelays[attempt]); err != nil {
			return nil, err
		}
	}
}

func (d *helmArtifactDownloader) downloadOnce(ctx context.Context, rawURL string) ([]byte, error) {
	attemptCtx := ctx
	cancel := func() {}
	if d.attemptTimeout > 0 {
		attemptCtx, cancel = context.WithTimeout(ctx, d.attemptTimeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Helm/3.21.2")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &helmHTTPStatusError{statusCode: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

func isRetryableHelmDownloadError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()) {
		return true
	}
	var statusError *helmHTTPStatusError
	if errors.As(err, &statusError) {
		return statusError.statusCode == http.StatusRequestTimeout ||
			statusError.statusCode == http.StatusTooManyRequests ||
			(statusError.statusCode >= http.StatusInternalServerError && statusError.statusCode < 600)
	}
	return false
}

func waitForHelmDownloadRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryOCIRegistryOperation(ctx context.Context, operation func() error) error {
	return retryOCIRegistryOperationWithDelays(ctx, operation, []time.Duration{
		500 * time.Millisecond,
		1500 * time.Millisecond,
	})
}

func retryOCIRegistryOperationWithDelays(ctx context.Context, operation func() error, delays []time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for attempt := 0; ; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil || attempt >= len(delays) || !isRetryableOCIRegistryError(err) {
			return err
		}
		if err := waitForHelmDownloadRetry(ctx, delays[attempt]); err != nil {
			return err
		}
	}
}

func isRetryableOCIRegistryError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()) {
		return true
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		return true
	}
	var responseError *errcode.ErrorResponse
	if errors.As(err, &responseError) {
		return responseError.StatusCode == http.StatusRequestTimeout ||
			responseError.StatusCode == http.StatusTooManyRequests ||
			(responseError.StatusCode >= http.StatusInternalServerError && responseError.StatusCode < 600)
	}
	return false
}

type helmArtifactCacheEntry struct {
	data      []byte
	storedAt  time.Time
	expiresAt time.Time
}

type helmArtifactCache struct {
	mu         sync.Mutex
	entries    map[string]helmArtifactCacheEntry
	ttl        time.Duration
	maxEntries int
	maxBytes   int
	usedBytes  int
}

func newHelmArtifactCache(ttl time.Duration, maxEntries, maxBytes int) *helmArtifactCache {
	return &helmArtifactCache{
		entries:    make(map[string]helmArtifactCacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
	}
}

func (c *helmArtifactCache) get(key string) ([]byte, bool) {
	if c == nil || c.ttl <= 0 || c.maxEntries <= 0 || c.maxBytes <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.deleteLocked(key, entry)
		return nil, false
	}
	return append([]byte(nil), entry.data...), true
}

func (c *helmArtifactCache) put(key string, data []byte) {
	if c == nil || c.ttl <= 0 || c.maxEntries <= 0 || c.maxBytes <= 0 || len(data) > c.maxBytes {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for existingKey, entry := range c.entries {
		if now.After(entry.expiresAt) {
			c.deleteLocked(existingKey, entry)
		}
	}
	if entry, ok := c.entries[key]; ok {
		c.deleteLocked(key, entry)
	}
	for len(c.entries) >= c.maxEntries || c.usedBytes+len(data) > c.maxBytes {
		oldestKey := ""
		var oldestEntry helmArtifactCacheEntry
		for existingKey, entry := range c.entries {
			if oldestKey == "" || entry.storedAt.Before(oldestEntry.storedAt) {
				oldestKey = existingKey
				oldestEntry = entry
			}
		}
		if oldestKey == "" {
			break
		}
		c.deleteLocked(oldestKey, oldestEntry)
	}
	cloned := append([]byte(nil), data...)
	c.entries[key] = helmArtifactCacheEntry{data: cloned, storedAt: now, expiresAt: now.Add(c.ttl)}
	c.usedBytes += len(cloned)
}

func (c *helmArtifactCache) deleteLocked(key string, entry helmArtifactCacheEntry) {
	delete(c.entries, key)
	c.usedBytes -= len(entry.data)
}
