package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// quoteChar is the double quote wrapping each field of tasklist CSV output.
const quoteChar = `"`

func getPidByPort(port int) (int, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "netstat -ano | findstr :"+strconv.Itoa(port))
	case "darwin", "linux":
		cmd = exec.Command("lsof", "-t", "-i", ":"+strconv.Itoa(port))
	default:
		return 0, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, nil
		}
		return 0, nil
	}

	portStr := strconv.Itoa(port)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if runtime.GOOS == "windows" {
			// match both 0.0.0.0:port and 127.0.0.1:port
			if len(fields) >= 2 && strings.HasSuffix(fields[1], ":"+portStr) {
				pid, err := strconv.Atoi(fields[len(fields)-1])
				if err != nil {
					return 0, err
				}
				return pid, nil
			}
		} else {
			pid, err := strconv.Atoi(fields[0])
			if err != nil {
				return 0, err
			}
			return pid, nil
		}
	}

	return 0, nil
}

// processImageName returns the executable name of a running process, without
// its directory. An empty name is returned when the process is gone or the
// platform lookup fails, which callers must read as "not known to be ours".
func processImageName(pid int) string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	case "darwin", "linux":
		cmd = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseProcessImageName(string(output), runtime.GOOS)
}

// parseProcessImageName pulls the executable name out of the tasklist or ps
// output for a single process.
func parseProcessImageName(output, goos string) string {
	line := strings.TrimSpace(output)
	if line == "" {
		return ""
	}
	line = strings.TrimSpace(strings.SplitN(line, "\n", 2)[0])
	if goos == "windows" {
		// tasklist prints one CSV record per match, image name first. A PID
		// with no match prints an INFO line carrying no quoted field at all.
		fields := strings.Split(line, quoteChar)
		if len(fields) < 2 {
			return ""
		}
		return fields[1]
	}
	return filepath.Base(line)
}

// isOwnExecutable reports whether pid belongs to another instance of the
// program running right now.
func isOwnExecutable(pid int) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	name := processImageName(pid)
	if name == "" {
		return false
	}
	return strings.EqualFold(name, filepath.Base(self))
}

// StopOldInstance kills a leftover CasOS instance still holding port, which is
// what a restart runs into while the previous process is on its way out.
//
// Only a process running the same executable is killed. The port may just as
// well belong to an unrelated program -- a real etcd on 2379, say -- and
// taking that down to claim the port would destroy data CasOS does not own.
// Callers move aside to another port instead, so a port still occupied when
// this returns is not an error.
func StopOldInstance(port int) error {
	pid, err := getPidByPort(port)
	if err != nil {
		return err
	}
	if pid == 0 || pid == os.Getpid() {
		return nil
	}
	if !isOwnExecutable(pid) {
		return nil
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err = process.Kill(); err != nil {
		return err
	}

	fmt.Printf("The old instance with pid: %d has been stopped\n", pid)
	return nil
}
