package deploy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const containerdCRISocketPath = "/run/containerd/containerd.sock"

type containerdCRIPullImageError struct {
	image string
	err   error
}

func (e *containerdCRIPullImageError) Error() string {
	return e.err.Error()
}

func (e *containerdCRIPullImageError) Unwrap() error {
	return e.err
}

func (e *containerdCRIPullImageError) containerdPullImage() string {
	return e.image
}

type sshSessionStdioConn struct {
	session   *ssh.Session
	reader    io.Reader
	writer    io.WriteCloser
	closeOnce sync.Once
}

func (c *sshSessionStdioConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *sshSessionStdioConn) Write(p []byte) (int, error) { return c.writer.Write(p) }

func (c *sshSessionStdioConn) Close() error {
	var closeErr error
	c.closeOnce.Do(func() {
		_ = c.writer.Close()
		closeErr = c.session.Close()
	})
	return closeErr
}

func (c *sshSessionStdioConn) LocalAddr() net.Addr { return containerdCRIAddr("casos") }
func (c *sshSessionStdioConn) RemoteAddr() net.Addr {
	return containerdCRIAddr(containerdCRISocketPath)
}
func (c *sshSessionStdioConn) SetDeadline(time.Time) error      { return nil }
func (c *sshSessionStdioConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sshSessionStdioConn) SetWriteDeadline(time.Time) error { return nil }

type containerdCRIAddr string

func (a containerdCRIAddr) Network() string { return "ssh-stdio" }
func (a containerdCRIAddr) String() string  { return string(a) }

func (r *NodeDeploySSHRunner) openContainerdCRIConnection(ctx context.Context) (net.Conn, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("open containerd CRI connection: SSH runner is required")
	}
	session, err := r.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open containerd CRI SSH session: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("open containerd CRI stdin: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		_ = session.Close()
		return nil, fmt.Errorf("open containerd CRI stdout: %w", err)
	}
	var stderr lockedBuffer
	session.Stderr = &stderr
	command := "if [ \"$(id -u)\" = 0 ]; then exec socat STDIO UNIX-CONNECT:" + containerdCRISocketPath + "; else exec sudo -n socat STDIO UNIX-CONNECT:" + containerdCRISocketPath + "; fi"
	waitCh, err := r.startSessionWithContext(ctx, session, command)
	if err != nil {
		_ = stdin.Close()
		_ = session.Close()
		return nil, fmt.Errorf("start containerd CRI tunnel: %w", err)
	}
	conn := &sshSessionStdioConn{session: session, reader: stdout, writer: stdin}
	go func() {
		<-waitCh
		_ = conn.Close()
	}()
	return conn, nil
}

func (r *NodeDeploySSHRunner) VerifyContainerdImagePullCRIContext(ctx context.Context, image string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	image = normalizeContainerdPullImage(image)
	if image == "" {
		return fmt.Errorf("verify containerd CRI image pull: image is required")
	}
	client, err := grpc.NewClient(
		"passthrough:///containerd-cri",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// The gRPC dial context ends after connection setup. Keep the SSH
		// session bound to the full verification context instead.
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return r.openContainerdCRIConnection(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("create containerd CRI client: %w", err)
	}
	defer client.Close()

	runtimeClient := runtimeapi.NewRuntimeServiceClient(client)
	if _, err := runtimeClient.Version(ctx, &runtimeapi.VersionRequest{Version: "0.1.0"}); err != nil {
		return fmt.Errorf("query containerd CRI version: %w", err)
	}
	imageClient := runtimeapi.NewImageServiceClient(client)
	imageSpec := &runtimeapi.ImageSpec{Image: image}
	var pullErr error
	for attempt := 1; attempt <= 3; attempt++ {
		response, err := imageClient.PullImage(ctx, &runtimeapi.PullImageRequest{Image: imageSpec})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("pull containerd CRI image %s: %w", image, ctxErr)
		}
		if err == nil && response.GetImageRef() != "" {
			return nil
		}
		if err == nil {
			err = fmt.Errorf("runtime returned an empty image reference")
		}
		pullErr = err
		if attempt == 3 {
			break
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("pull containerd CRI image %s: %w", image, ctx.Err())
		case <-timer.C:
		}
	}
	return &containerdCRIPullImageError{
		image: image,
		err:   fmt.Errorf("pull containerd CRI image %s after 3 attempts: %w", image, pullErr),
	}
}
