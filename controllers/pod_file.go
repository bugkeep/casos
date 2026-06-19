package controllers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

type fileEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "dir" | "file" | "link"
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	ModTime     string `json:"modTime"`
}

// lsLineRe matches a line of `ls -la` output.
// Groups: perms, size, month, day, time-or-year, name
var lsLineRe = regexp.MustCompile(
	`^([dlrwxsStT-]{10})\s+\d+\s+\S+\s+\S+\s+(\d+)\s+(\w+\s+\d+)\s+(\S+)\s+(.+)$`,
)

func parseLsOutput(output string) []fileEntry {
	var entries []fileEntry
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "total ") {
			continue
		}
		m := lsLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		perms := m[1]
		size, _ := strconv.ParseInt(strings.TrimSpace(m[2]), 10, 64)
		monthDay := strings.TrimSpace(m[3])
		timeOrYear := strings.TrimSpace(m[4])
		name := strings.TrimSpace(m[5])

		// Strip symlink target " -> ..."
		if idx := strings.Index(name, " -> "); idx >= 0 {
			name = name[:idx]
		}
		if name == "." || name == ".." {
			continue
		}

		entryType := "file"
		switch perms[0] {
		case 'd':
			entryType = "dir"
		case 'l':
			entryType = "link"
		}

		entries = append(entries, fileEntry{
			Name:        name,
			Type:        entryType,
			Size:        size,
			Permissions: perms,
			ModTime:     monthDay + " " + timeOrYear,
		})
	}
	return entries
}

// ListPodFiles returns the directory listing inside a container.
// Query: namespace, name, container, path
// @router /api/pod-file-list [get]
func (c *ApiController) ListPodFiles() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	name := c.GetString("name")
	container := c.GetString("container")
	dirPath := c.GetString("path")
	if namespace == "" {
		namespace = "default"
	}
	if dirPath == "" {
		dirPath = "/"
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	var stdout, stderr bytes.Buffer
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(name).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"ls", "-la", dirPath},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := exec.StreamWithContext(c.Ctx.Request.Context(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		c.ResponseError(msg)
		return
	}

	entries := parseLsOutput(stdout.String())
	if entries == nil {
		entries = []fileEntry{}
	}
	c.ResponseOk(entries)
}

// DownloadPodFile streams a single file from a container as an attachment.
// Query: namespace, name, container, path
// @router /api/pod-file-download [get]
func (c *ApiController) DownloadPodFile() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	name := c.GetString("name")
	container := c.GetString("container")
	filePath := c.GetString("path")

	if namespace == "" {
		namespace = "default"
	}
	if filePath == "" {
		c.ResponseError("path is required")
		return
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	dir := path.Dir(filePath)
	base := path.Base(filePath)

	var stdout, stderr bytes.Buffer
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(name).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"tar", "cf", "-", "-C", dir, base},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := exec.StreamWithContext(c.Ctx.Request.Context(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		c.ResponseError(msg)
		return
	}

	tr := tar.NewReader(&stdout)
	hdr, err := tr.Next()
	if err != nil {
		c.ResponseError("failed to read tar: " + err.Error())
		return
	}

	c.Ctx.ResponseWriter.Header().Set("Content-Type", "application/octet-stream")
	c.Ctx.ResponseWriter.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"`, path.Base(hdr.Name)))
	c.Ctx.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", hdr.Size))
	c.Ctx.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = io.Copy(c.Ctx.ResponseWriter, tr)
}

// UploadPodFile uploads a file into a container at the specified path.
// Multipart: namespace, name, container, destDir, file
// @router /api/pod-file-upload [post]
func (c *ApiController) UploadPodFile() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	name := c.GetString("name")
	container := c.GetString("container")
	destDir := c.GetString("destDir")

	if namespace == "" {
		namespace = "default"
	}
	if destDir == "" {
		destDir = "/tmp"
	}

	file, fileHeader, err := c.GetFile("file")
	if err != nil {
		c.ResponseError("no file in request: " + err.Error())
		return
	}
	defer file.Close()

	filename := path.Base(fileHeader.Filename)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.ResponseError("failed to read upload: " + err.Error())
		return
	}
	if err := tw.WriteHeader(&tar.Header{
		Name: filename,
		Mode: 0o644,
		Size: int64(len(fileBytes)),
	}); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if _, err := tw.Write(fileBytes); err != nil {
		c.ResponseError(err.Error())
		return
	}
	tw.Close()

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	var stderr bytes.Buffer
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(name).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"tar", "xf", "-", "-C", destDir},
			Stdin:     true,
			Stdout:    false,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := exec.StreamWithContext(c.Ctx.Request.Context(), remotecommand.StreamOptions{
		Stdin:  &tarBuf,
		Stderr: &stderr,
	}); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		c.ResponseError(msg)
		return
	}

	c.ResponseOk(fmt.Sprintf("%s/%s", strings.TrimRight(destDir, "/"), filename))
}
