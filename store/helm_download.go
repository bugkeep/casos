package store

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	proxypkg "github.com/casosorg/casos/proxy"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

const (
	helmDownloadAttemptTimeout      = 2 * time.Minute
	helmRegistryConnectTimeout      = 30 * time.Second
	helmRegistryTLSHandshakeTimeout = 30 * time.Second
	helmDownloadCacheTTL            = 15 * time.Minute
	helmDownloadCacheEntries        = 32
	helmDownloadCacheBytes          = 128 << 20
)

var defaultHelmArtifactCache = newHelmArtifactCache(helmDownloadCacheTTL, helmDownloadCacheEntries, helmDownloadCacheBytes)

func newOCIRegistryHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clonedClient := *client
	clonedClient.Timeout = 0

	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if httpTransport, ok := transport.(*http.Transport); ok {
		clonedTransport := httpTransport.Clone()
		configureOCIRegistryDialTimeout(clonedTransport, helmRegistryConnectTimeout)
		clonedTransport.TLSHandshakeTimeout = helmRegistryTLSHandshakeTimeout
		clonedTransport.ResponseHeaderTimeout = helmDownloadAttemptTimeout
		clonedClient.Transport = clonedTransport
	}
	return &clonedClient
}

func configureOCIRegistryDialTimeout(transport *http.Transport, timeout time.Duration) {
	if transport == nil || timeout <= 0 {
		return
	}
	if transport.DialContext != nil {
		dialContext := transport.DialContext
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return dialContext(dialCtx, network, address)
		}
		return
	}
	if transport.Dial != nil {
		dial := transport.Dial
		transport.Dial = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			type dialResult struct {
				conn net.Conn
				err  error
			}
			resultCh := make(chan dialResult)
			go func() {
				result := dialResult{}
				result.conn, result.err = dial(network, address)
				select {
				case resultCh <- result:
				case <-dialCtx.Done():
					if result.conn != nil {
						_ = result.conn.Close()
					}
				}
			}()
			select {
			case result := <-resultCh:
				return result.conn, result.err
			case <-dialCtx.Done():
				return nil, dialCtx.Err()
			}
		}
		return
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport.DialContext = dialer.DialContext
}

func downloadHelmArtifact(ctx context.Context, rawURL string) ([]byte, error) {
	downloader := newHelmArtifactDownloader(
		proxypkg.ProxyHttpClient,
		helmDownloadAttemptTimeout,
		[]time.Duration{time.Second, 3 * time.Second},
		defaultHelmArtifactCache,
	)
	return downloader.download(ctx, rawURL)
}

func downloadHelmRepoIndex(ctx context.Context, rawURL string) ([]byte, error) {
	downloader := newHelmArtifactDownloader(
		proxypkg.ProxyHttpClient,
		helmDownloadAttemptTimeout,
		[]time.Duration{time.Second, 3 * time.Second},
		defaultHelmArtifactCache,
	)
	downloader.revalidate = true
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
	revalidate     bool
}

type helmArtifactDownload struct {
	data         []byte
	etag         string
	lastModified string
	notModified  bool
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
	var cached *helmArtifactCacheEntry
	if d.cache != nil {
		if entry, ok := d.cache.getEntry(rawURL); ok {
			if !d.revalidate {
				return entry.data, nil
			}
			cached = &entry
		}
	}

	for attempt := 0; ; attempt++ {
		result, err := d.downloadOnce(ctx, rawURL, cached)
		if err == nil {
			if result.notModified {
				if cached == nil {
					return nil, fmt.Errorf("HTTP %d without cached data", http.StatusNotModified)
				}
				if d.cache != nil {
					d.cache.refresh(rawURL)
				}
				return cached.data, nil
			}
			if d.cache != nil {
				d.cache.putWithMetadata(rawURL, result.data, result.etag, result.lastModified)
			}
			return result.data, nil
		}
		if ctx.Err() != nil || attempt >= len(d.retryDelays) || !isRetryableHelmDownloadError(err) {
			return nil, err
		}
		if err := waitForHelmDownloadRetry(ctx, d.retryDelays[attempt]); err != nil {
			return nil, err
		}
	}
}

func (d *helmArtifactDownloader) downloadOnce(ctx context.Context, rawURL string, cached *helmArtifactCacheEntry) (*helmArtifactDownload, error) {
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
	// Asking for gzip explicitly turns off the transport's transparent decoding,
	// keeping Content-Length and the bytes read in the same units.
	req.Header.Set("Accept-Encoding", "gzip")
	if cached != nil {
		if cached.etag != "" {
			req.Header.Set("If-None-Match", cached.etag)
		}
		if cached.lastModified != "" {
			req.Header.Set("If-Modified-Since", cached.lastModified)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return &helmArtifactDownload{notModified: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &helmHTTPStatusError{statusCode: resp.StatusCode}
	}
	data, err := readHelmArtifactBody(attemptCtx, resp, rawURL)
	if err != nil {
		return nil, err
	}
	return &helmArtifactDownload{
		data:         data,
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// readHelmArtifactBody returns the decoded response body, counting the encoded
// bytes as they arrive so a slow repository shows progress instead of silence.
func readHelmArtifactBody(ctx context.Context, resp *http.Response, rawURL string) ([]byte, error) {
	var body io.Reader = resp.Body
	if progress := newHelmChartLoadProgressReader(ctx, rawURL, resp.ContentLength); progress != nil {
		defer progress.Finish()
		body = io.TeeReader(body, progress)
	}
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("decompress gzip response: %w", err)
		}
		defer gzipReader.Close()
		body = gzipReader
	}
	return io.ReadAll(body)
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
		if attempt >= len(delays) || !isRetryableOCIRegistryError(err) {
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
	data         []byte
	etag         string
	lastModified string
	lastAccess   uint64
	expiresAt    time.Time
}

type helmArtifactCache struct {
	mu         sync.Mutex
	entries    map[string]helmArtifactCacheEntry
	ttl        time.Duration
	maxEntries int
	maxBytes   int
	usedBytes  int
	accessSeq  uint64
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
	entry, ok := c.getEntry(key)
	if !ok {
		return nil, false
	}
	return entry.data, true
}

func (c *helmArtifactCache) getEntry(key string) (helmArtifactCacheEntry, bool) {
	if c == nil || c.ttl <= 0 || c.maxEntries <= 0 || c.maxBytes <= 0 {
		return helmArtifactCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return helmArtifactCacheEntry{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.deleteLocked(key, entry)
		return helmArtifactCacheEntry{}, false
	}
	c.accessSeq++
	entry.lastAccess = c.accessSeq
	c.entries[key] = entry
	// Cache entries are immutable after insertion; callers must treat this
	// shared slice as read-only so large indexes and charts are not copied.
	return entry, true
}

func (c *helmArtifactCache) put(key string, data []byte) {
	c.putWithMetadata(key, data, "", "")
}

func (c *helmArtifactCache) putWithMetadata(key string, data []byte, etag, lastModified string) {
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
		leastRecentlyUsedKey := ""
		var leastRecentlyUsedEntry helmArtifactCacheEntry
		for existingKey, entry := range c.entries {
			if leastRecentlyUsedKey == "" || entry.lastAccess < leastRecentlyUsedEntry.lastAccess {
				leastRecentlyUsedKey = existingKey
				leastRecentlyUsedEntry = entry
			}
		}
		if leastRecentlyUsedKey == "" {
			break
		}
		c.deleteLocked(leastRecentlyUsedKey, leastRecentlyUsedEntry)
	}
	cloned := append([]byte(nil), data...)
	c.accessSeq++
	c.entries[key] = helmArtifactCacheEntry{
		data:         cloned,
		etag:         etag,
		lastModified: lastModified,
		lastAccess:   c.accessSeq,
		expiresAt:    now.Add(c.ttl),
	}
	c.usedBytes += len(cloned)
}

func (c *helmArtifactCache) refresh(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return
	}
	c.accessSeq++
	entry.lastAccess = c.accessSeq
	entry.expiresAt = time.Now().Add(c.ttl)
	c.entries[key] = entry
}

func (c *helmArtifactCache) deleteLocked(key string, entry helmArtifactCacheEntry) {
	delete(c.entries, key)
	c.usedBytes -= len(entry.data)
}
