package server

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHRunner interface {
	Run(command string) (string, string, error)
	WriteFile(path, content string, mode int) error
	AppendAuthorizedKey(publicKey string) error
	Close() error
}

type remoteSSHRunner struct {
	client *ssh.Client
}

func NewSSHRunner(host string, port int, username, password string) (SSHRunner, error) {
	return newSSHRunner(host, port, username, ssh.Password(password))
}

func NewSSHRunnerWithPrivateKey(host string, port int, username, privateKey string) (SSHRunner, error) {
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		return nil, err
	}
	return newSSHRunner(host, port, username, ssh.PublicKeys(signer))
}

func newSSHRunner(host string, port int, username string, auth ssh.AuthMethod) (SSHRunner, error) {
	config := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         20 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
	if err != nil {
		return nil, err
	}
	return &remoteSSHRunner{client: client}, nil
}

func (r *remoteSSHRunner) Run(command string) (string, string, error) {
	session, err := r.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func (r *remoteSSHRunner) WriteFile(path, content string, mode int) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	command := fmt.Sprintf("install -d -m 0755 $(dirname %q) && printf '%%s' %q | base64 -d > %q && chmod %o %q", path, encoded, path, mode, path)
	_, stderr, err := r.Run(command)
	if err != nil {
		if stderr != "" {
			return fmt.Errorf("%w: %s", err, stderr)
		}
		return err
	}
	return nil
}

func (r *remoteSSHRunner) AppendAuthorizedKey(publicKey string) error {
	command := fmt.Sprintf("install -d -m 0700 ~/.ssh && touch ~/.ssh/authorized_keys && chmod 0600 ~/.ssh/authorized_keys && grep -qxF %q ~/.ssh/authorized_keys || printf '%%s\\n' %q >> ~/.ssh/authorized_keys", publicKey, publicKey)
	_, stderr, err := r.Run(command)
	if err != nil {
		return commandError("append authorized key", stderr, err)
	}
	return nil
}

func (r *remoteSSHRunner) Close() error {
	return r.client.Close()
}

func GenerateManagedNodeSSHKeyPair() (string, string, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return "", "", err
	}
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})
	return string(privatePEM), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPublicKey))), nil
}
