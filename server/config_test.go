package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kinesqlite "github.com/k3s-io/kine/pkg/drivers/sqlite"
	"github.com/k3s-io/kine/pkg/endpoint"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestKineEndpointConfigListensOnTheGivenPort(t *testing.T) {
	kineConfig := kineEndpointConfig("sqlite://casos.db", 12345)

	want := "tcp://127.0.0.1:12345"
	if kineConfig.Listener != want {
		t.Errorf("Listener = %q, want %q", kineConfig.Listener, want)
	}
}

func TestResolveDatastoreEndpoint(t *testing.T) {
	dataDir := t.TempDir()

	t.Run("SQLite uses dedicated state file", func(t *testing.T) {
		got, err := resolveDatastoreEndpoint("sqlite", filepath.Join(dataDir, "casos.db"), "casos", dataDir, "")
		if err != nil {
			t.Fatalf("resolveDatastoreEndpoint() error = %v", err)
		}
		wantPrefix := "sqlite://" + filepath.ToSlash(filepath.Join(dataDir, "kine", "state.db")) + "?"
		if !strings.HasPrefix(got, wantPrefix) {
			t.Fatalf("endpoint = %q, want prefix %q", got, wantPrefix)
		}
		if !strings.HasSuffix(got, kinesqlite.DefaultParams) {
			t.Fatalf("endpoint = %q, want Kine SQLite defaults", got)
		}
	})

	t.Run("MySQL remains supported", func(t *testing.T) {
		got, err := resolveDatastoreEndpoint("mysql", "root:secret@tcp(localhost:3306)/", "casos", dataDir, "")
		if err != nil {
			t.Fatalf("resolveDatastoreEndpoint() error = %v", err)
		}
		want := "mysql://root:secret@tcp(localhost:3306)/casos"
		if got != want {
			t.Fatalf("endpoint = %q, want %q", got, want)
		}
	})

	t.Run("MySQL without a DSN is rejected", func(t *testing.T) {
		if _, err := resolveDatastoreEndpoint("mysql", "  ", "casos", dataDir, ""); err == nil {
			t.Fatal("resolveDatastoreEndpoint() error = nil, want an error for an empty MySQL DSN")
		}
	})

	t.Run("explicit endpoint takes precedence", func(t *testing.T) {
		const endpoint = "postgres://postgres:secret@localhost/casos"
		got, err := resolveDatastoreEndpoint("postgres", "", "casos", dataDir, endpoint)
		if err != nil {
			t.Fatalf("resolveDatastoreEndpoint() error = %v", err)
		}
		if got != endpoint {
			t.Fatalf("endpoint = %q, want %q", got, endpoint)
		}
	})
}

func TestKineSQLiteEndpoint(t *testing.T) {
	dataDir := tempDataDir(t)
	datastoreEndpoint, err := resolveDatastoreEndpoint("sqlite", "", "casos", dataDir, "")
	if err != nil {
		t.Fatalf("resolveDatastoreEndpoint() error = %v", err)
	}
	if err := ensureKineDataDirectory(datastoreEndpoint); err != nil {
		t.Fatalf("ensureKineDataDirectory() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	stopKine := func() {
		cancel()
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for Kine shutdown")
		}
	}

	kineConfig := kineEndpointConfig(datastoreEndpoint, kineDefaultPort)
	kineConfig.Listener = "tcp://127.0.0.1:0"
	kineConfig.WaitGroup = &wg
	etcdConfig, err := endpoint.Listen(ctx, kineConfig)
	if err != nil {
		cancel()
		t.Fatalf("start Kine: %v", err)
	}
	defer stopKine()

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdConfig.Endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create etcd client: %v", err)
	}
	defer client.Close()

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer requestCancel()

	if _, err = client.Put(requestCtx, "/casos/test", "value"); err != nil {
		t.Fatalf("write through Kine: %v", err)
	}
	response, err := client.Get(requestCtx, "/casos/test")
	if err != nil {
		t.Fatalf("read through Kine: %v", err)
	}
	if len(response.Kvs) != 1 || string(response.Kvs[0].Value) != "value" {
		t.Fatalf("unexpected Kine response: %+v", response.Kvs)
	}

	if _, err = client.Put(requestCtx, kubernetesEndpointsEtcdKey, "stale"); err != nil {
		t.Fatalf("seed stale Kubernetes Endpoints: %v", err)
	}
	if err = deleteStaleKubernetesEndpoints(requestCtx, etcdConfig.Endpoints[0]); err != nil {
		t.Fatalf("delete stale Kubernetes Endpoints: %v", err)
	}
	response, err = client.Get(requestCtx, kubernetesEndpointsEtcdKey)
	if err != nil {
		t.Fatalf("read deleted Kubernetes Endpoints: %v", err)
	}
	if len(response.Kvs) != 0 {
		t.Fatalf("stale Kubernetes Endpoints still exists: %+v", response.Kvs)
	}

	statePath := filepath.Join(dataDir, "kine", "state.db")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("stat Kine state database: %v", err)
	}
}

// tempDataDir is t.TempDir() with a best-effort cleanup. Kine does not release
// the SQLite file handle synchronously on shutdown, and on Windows that makes
// the removal registered by t.TempDir() fail the test even though the
// assertions passed.
func tempDataDir(t *testing.T) string {
	t.Helper()
	dataDir, err := os.MkdirTemp("", "casos-kine-")
	if err != nil {
		t.Fatalf("create temporary data directory: %v", err)
	}
	t.Cleanup(func() {
		for attempt := 0; attempt < 10; attempt++ {
			if err := os.RemoveAll(dataDir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("could not remove temporary data directory %s", dataDir)
	})
	return dataDir
}
