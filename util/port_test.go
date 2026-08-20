package util

import (
	"net"
	"strconv"
	"testing"
)

func listenOn(t *testing.T, port int) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("occupy port %d: %v", port, err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func TestFreePortFromKeepsAnUnoccupiedPort(t *testing.T) {
	preferred := freePort(t)

	port, err := FreePortFrom("127.0.0.1", preferred)
	if err != nil {
		t.Fatalf("FreePortFrom: %v", err)
	}
	if port != preferred {
		t.Errorf("port = %d, want the unoccupied preferred port %d", port, preferred)
	}
}

func TestFreePortFromSkipsOccupiedPorts(t *testing.T) {
	preferred := freePort(t)
	listenOn(t, preferred)
	listenOn(t, preferred+1)

	port, err := FreePortFrom("127.0.0.1", preferred)
	if err != nil {
		t.Fatalf("FreePortFrom: %v", err)
	}
	if port != preferred+2 {
		t.Errorf("port = %d, want %d — the first port above the occupied ones", port, preferred+2)
	}
}

func TestFreePortFromRejectsAnOutOfRangePort(t *testing.T) {
	for _, preferred := range []int{0, -1, 65536} {
		if _, err := FreePortFrom("127.0.0.1", preferred); err == nil {
			t.Errorf("FreePortFrom(%d) = nil error, want an out of range error", preferred)
		}
	}
}

func TestPortAvailableReportsAnOccupiedPort(t *testing.T) {
	port := freePort(t)
	if !PortAvailable("127.0.0.1", port) {
		t.Fatalf("PortAvailable(%d) = false before anything listens on it", port)
	}

	listenOn(t, port)
	if PortAvailable("127.0.0.1", port) {
		t.Errorf("PortAvailable(%d) = true while a listener holds it", port)
	}
}
