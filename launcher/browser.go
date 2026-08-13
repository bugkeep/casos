package launcher

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"time"
)

func OpenWhenReady(ctx context.Context, address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for %s", parsed.Host)
		case <-ticker.C:
			connection, dialErr := net.DialTimeout("tcp", parsed.Host, 250*time.Millisecond)
			if dialErr != nil {
				continue
			}
			_ = connection.Close()
			return open(address)
		}
	}
}

func open(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	return command.Start()
}
