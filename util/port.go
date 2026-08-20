package util

import (
	"fmt"
	"net"
	"strconv"
)

// portScanLimit caps how far FreePortFrom walks up from its preferred port. A
// machine that has the next thirty-two ports occupied has something more wrong
// with it than CasOS can paper over, and walking forever would hide that.
const portScanLimit = 32

// PortAvailable reports whether a TCP listener can be opened on bind:port.
//
// The answer is a snapshot: the listener is closed again before returning, so
// another process can take the port between the check and the real bind. It is
// only meant for picking a starting point for a component that binds the port
// itself moments later.
func PortAvailable(bind string, port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// FreePortFrom returns the first free TCP port at or above preferred on bind,
// so a component whose port is taken by an unrelated program moves aside
// instead of failing to start.
//
// It is for ports nothing outside the process dials — the caller passes the
// result straight to the component it starts, and no configuration file, node
// or stored object records the number. A port other machines connect to has to
// stay where it was instead.
func FreePortFrom(bind string, preferred int) (int, error) {
	if preferred < 1 || preferred > 65535 {
		return 0, fmt.Errorf("port %d out of range", preferred)
	}
	for port := preferred; port < preferred+portScanLimit && port <= 65535; port++ {
		if PortAvailable(bind, port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port in %d-%d on %s", preferred, preferred+portScanLimit-1, bind)
}
