package deploy

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
)

const (
	localHostMachineFallbackName = "localhost"
	localHostMachineAddress      = "127.0.0.1"
)

// AddLocalHostMachine registers the CasOS host itself as a machine, so a Linux
// host can become a worker node of its own cluster with nothing to configure.
// The record carries no credential: the deployment reaches it through a local
// shell rather than SSH. Running it again refreshes the record.
func AddLocalHostMachine(ctx context.Context, owner string) (*object.Machine, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "admin"
	}
	runner, err := NewNodeDeployLocalRunner()
	if err != nil {
		return nil, err
	}
	defer runner.Close()

	hostname, err := os.Hostname()
	if err != nil {
		logs.Warning("failed to read the hostname of this CasOS host: %v", err)
	}
	name := localHostMachineName(hostname)

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
			DisplayName: localHostMachineDisplayName(hostname),
			Description: "The CasOS host, enrolled by CasOS",
		}
	}
	machine.Ip = localHostMachineAddress
	machine.Port = 0
	machine.Username = localHostUsername()
	machine.AuthType = object.MachineAuthTypeLocal
	machine.Password = ""
	machine.PrivateKey = ""
	machine.Os = localHostOS(ctx, runner)
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

	logs.Info("enrolled the CasOS host as machine %s/%s (%s)", owner, name, machine.Os)
	return machine, nil
}

func localHostMachineName(hostname string) string {
	name := sanitizeMachineName(hostname, localHostMachineFallbackName)
	if len(name) > 100 {
		name = strings.TrimRight(name[:100], "-")
	}
	return name
}

func localHostMachineDisplayName(hostname string) string {
	if hostname = strings.TrimSpace(hostname); hostname != "" {
		return hostname
	}
	return "CasOS host"
}

// localHostUsername records who CasOS runs as. It is informational: a local
// machine is never logged in to.
func localHostUsername() string {
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return ""
}

func localHostOS(ctx context.Context, runner NodeDeployRunner) string {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := runner.RunContext(probeCtx, `. /etc/os-release >/dev/null 2>&1; printf %s "${PRETTY_NAME:-${NAME:-}}"`)
	if err == nil {
		if name := strings.TrimSpace(out); name != "" {
			return name
		}
	}
	return runtime.GOOS
}

// sanitizeMachineName turns a hostname or distro name into a machine name that
// passes the same validation as a manually added machine.
func sanitizeMachineName(value, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(collapseDashes(b.String()), "-")
	if name == "" {
		return fallback
	}
	return name
}
