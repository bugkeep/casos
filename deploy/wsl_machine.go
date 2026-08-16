package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/wsl"
)

const (
	localWSLMachineNamePrefix = "wsl-"
	localWSLProvisionTimeout  = 5 * time.Minute
	localWSLProbeTimeout      = 8 * time.Second
)

// LocalWSLMachineResult reports what happened while enrolling the local WSL distro.
type LocalWSLMachineResult struct {
	Machine *object.Machine `json:"machine"`
	Distro  string          `json:"distro"`
	Created bool            `json:"created"`
	// Sudo is true when the enrolled user reaches root through sudo instead of
	// already being root.
	Sudo bool `json:"sudo"`
}

// AddLocalWSLMachine registers a WSL distro of the local Windows host as a
// machine. An empty distro selects the default one. It provisions sshd inside
// the distro, installs a generated key pair and verifies the connection, so the
// caller needs no connection details. Running it again for the same distro
// refreshes the address and credential, which is needed because WSL changes its
// IP address on every restart.
func AddLocalWSLMachine(ctx context.Context, owner, distro string) (*LocalWSLMachineResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "admin"
	}
	if err := wsl.Available(); err != nil {
		return nil, err
	}
	distro = strings.TrimSpace(distro)
	if distro == "" {
		// The default distro is not always the right one: Docker Desktop and
		// friends register distros that cannot host a node, and one of them may
		// well be the default.
		if status, err := wsl.Detect(ctx); err == nil {
			if selected := status.NodeDistro(); selected != nil {
				distro = selected.Name
			}
		}
	}

	keyPair, err := GenerateNodeDeployKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate SSH key pair: %w", err)
	}

	provisionCtx, cancel := context.WithTimeout(ctx, localWSLProvisionTimeout)
	defer cancel()
	provisioned, err := wsl.Provision(provisionCtx, distro, keyPair.PublicKey)
	if err != nil {
		return nil, err
	}

	endpoint, err := probeLocalWSLEndpoint(ctx, provisioned, keyPair.PrivateKey)
	if err != nil {
		if len(provisioned.Warnings) > 0 {
			return nil, fmt.Errorf("%w (%s)", err, strings.Join(provisioned.Warnings, "; "))
		}
		return nil, err
	}

	name := localWSLMachineName(provisioned.Distro)
	machine, err := object.GetMachine(fmt.Sprintf("%s/%s", owner, name))
	if err != nil {
		return nil, err
	}

	created := machine == nil
	if created {
		machine = &object.Machine{
			Owner:       owner,
			Name:        name,
			CreatedTime: time.Now().UTC().Format(time.RFC3339),
			DisplayName: localWSLDisplayName(provisioned.Distro),
			Description: "Local WSL distro enrolled by CasOS",
		}
	}
	machine.Ip = endpoint.host
	machine.Port = provisioned.Port
	machine.Username = endpoint.username
	machine.AuthType = "privateKey"
	machine.Password = ""
	machine.PrivateKey = keyPair.PrivateKey
	machine.Os = provisioned.Os
	if machine.Status != object.MachineStatusDeployed {
		machine.Status = "Online"
	}

	if created {
		if _, err = object.AddMachine(machine); err != nil {
			return nil, err
		}
	} else if _, err = object.UpdateMachine(fmt.Sprintf("%s/%s", owner, name), machine); err != nil {
		return nil, err
	}

	logs.Info("enrolled local WSL distro %s as machine %s/%s (%s@%s:%d)",
		provisioned.Distro, owner, name, endpoint.username, endpoint.host, provisioned.Port)

	// Never hand the private key back to API clients.
	response := *machine
	response.PrivateKey = ""
	return &LocalWSLMachineResult{
		Machine: &response,
		Distro:  provisioned.Distro,
		Created: created,
		Sudo:    endpoint.sudo,
	}, nil
}

type localWSLEndpoint struct {
	host     string
	username string
	sudo     bool
}

// probeLocalWSLEndpoint finds the address and user that actually accept the
// generated key. WSL is reachable on its own eth0 address, and on 127.0.0.1
// when localhost forwarding or mirrored networking is enabled.
func probeLocalWSLEndpoint(ctx context.Context, provisioned *wsl.ProvisionResult, privateKey string) (*localWSLEndpoint, error) {
	hosts := append([]string{}, provisioned.Hosts...)
	if !containsString(hosts, "127.0.0.1") {
		hosts = append(hosts, "127.0.0.1")
	}
	users := []string{}
	if provisioned.Username != "" {
		users = append(users, provisioned.Username)
	}
	if !containsString(users, "root") {
		users = append(users, "root")
	}

	var fallback *localWSLEndpoint
	failures := []string{}
	for _, host := range hosts {
		for _, user := range users {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			privileged, err := checkLocalWSLLogin(ctx, host, provisioned.Port, user, privateKey)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s@%s:%d: %v", user, host, provisioned.Port, err))
				continue
			}
			endpoint := &localWSLEndpoint{host: host, username: user, sudo: privileged == "SUDO"}
			if privileged == "ROOT" || privileged == "SUDO" {
				return endpoint, nil
			}
			if fallback == nil {
				fallback = endpoint
			}
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("could not reach sshd inside WSL: %s", strings.Join(failures, "; "))
}

// checkLocalWSLLogin returns ROOT, SUDO or NOROOT for a working SSH login.
func checkLocalWSLLogin(ctx context.Context, host string, port int, username, privateKey string) (string, error) {
	runner, err := NewNodeDeploySSHRunner(NodeDeploySSHConfig{
		Host:           host,
		Port:           port,
		Username:       username,
		PrivateKey:     privateKey,
		Timeout:        localWSLProbeTimeout,
		CommandTimeout: localWSLProbeTimeout,
	})
	if err != nil {
		return "", err
	}
	defer runner.Close()

	probeCtx, cancel := context.WithTimeout(ctx, localWSLProbeTimeout)
	defer cancel()
	out, err := runner.RunContext(probeCtx, `if [ "$(id -u)" = 0 ]; then echo ROOT; elif sudo -n true 2>/dev/null; then echo SUDO; else echo NOROOT; fi`)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// localWSLMachineName turns a distro name such as "Ubuntu-22.04" into a machine
// name that passes the same validation as manually added machines.
func localWSLMachineName(distro string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(distro)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(collapseDashes(b.String()), "-")
	if name == "" {
		name = "default"
	}
	name = localWSLMachineNamePrefix + name
	if len(name) > 100 {
		name = strings.TrimRight(name[:100], "-")
	}
	return name
}

func localWSLDisplayName(distro string) string {
	distro = strings.TrimSpace(distro)
	if distro == "" {
		return "Local WSL"
	}
	return "WSL: " + distro
}

func collapseDashes(value string) string {
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
