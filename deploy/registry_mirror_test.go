package deploy

import (
	"context"
	"strings"
	"testing"

	"github.com/casosorg/casos/server"
)

// fakeRegistryMirrorRunner answers the probe commands from a canned map and
// records what was written, so mirror selection can be exercised without a
// worker to run shell on.
type fakeRegistryMirrorRunner struct {
	reachable map[string]bool
	commands  []string
	written   map[string]string
}

func newFakeRegistryMirrorRunner(reachable map[string]bool) *fakeRegistryMirrorRunner {
	return &fakeRegistryMirrorRunner{reachable: reachable, written: map[string]string{}}
}

func (r *fakeRegistryMirrorRunner) RunRootContext(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	for url, reachable := range r.reachable {
		if strings.Contains(command, url) {
			if reachable {
				return "reachable", nil
			}
			return "unreachable:7", nil
		}
	}
	return "absent", nil
}

func (r *fakeRegistryMirrorRunner) WriteFileContext(_ context.Context, path, content, _ string) error {
	r.written[path] = content
	return nil
}

func testNodeDeployer(mode server.RegistryMirrorMode) *NodeDeployer {
	return NewNodeDeployer(Config{RegistryMirrorMode: mode, SandboxImage: "registry.k8s.io/pause:3.10.1"}, nil, nil)
}

func TestResolveRegistryMirrorsFollowsMode(t *testing.T) {
	for _, testCase := range []struct {
		mode server.RegistryMirrorMode
		want registryMirrorSelection
	}{
		{server.RegistryMirrorModeAlways, registryMirrorSelection{dockerHub: true, k8s: true, ghcr: true}},
		{server.RegistryMirrorModeNever, registryMirrorSelection{}},
	} {
		selection, err := testNodeDeployer(testCase.mode).resolveRegistryMirrors(context.Background(), newFakeRegistryMirrorRunner(nil))
		if err != nil {
			t.Fatalf("resolveRegistryMirrors(%s): %v", testCase.mode, err)
		}
		if selection != testCase.want {
			t.Fatalf("resolveRegistryMirrors(%s) = %+v, want %+v", testCase.mode, selection, testCase.want)
		}
	}
}

func TestResolveRegistryMirrorsAutoTiesGhcrToTheOthers(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		dockerHub bool
		k8s       bool
		want      registryMirrorSelection
	}{
		{"both reachable", true, true, registryMirrorSelection{}},
		{"docker hub blocked", false, true, registryMirrorSelection{dockerHub: true, ghcr: true}},
		{"registry.k8s.io blocked", true, false, registryMirrorSelection{k8s: true, ghcr: true}},
		{"both blocked", false, false, registryMirrorSelection{dockerHub: true, k8s: true, ghcr: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := newFakeRegistryMirrorRunner(map[string]bool{
				"registry-1.docker.io": testCase.dockerHub,
				"registry.k8s.io":      testCase.k8s,
			})
			selection, err := testNodeDeployer(server.RegistryMirrorModeAuto).resolveRegistryMirrors(context.Background(), runner)
			if err != nil {
				t.Fatalf("resolveRegistryMirrors: %v", err)
			}
			if selection != testCase.want {
				t.Fatalf("resolveRegistryMirrors = %+v, want %+v", selection, testCase.want)
			}
		})
	}
}

func TestReconcileRegistryMirrorFilesWritesGhcrMirror(t *testing.T) {
	runner := newFakeRegistryMirrorRunner(nil)
	deployer := testNodeDeployer(server.RegistryMirrorModeAuto)
	if err := deployer.reconcileRegistryMirrorFiles(context.Background(), runner, registryMirrorSelection{ghcr: true}); err != nil {
		t.Fatalf("reconcileRegistryMirrorFiles: %v", err)
	}

	content, ok := runner.written[ghcrHostsPath]
	if !ok {
		t.Fatalf("%s was not written; wrote %v", ghcrHostsPath, runner.written)
	}
	if !strings.HasPrefix(content, generatedRegistryHostsMarker) {
		t.Fatalf("%s does not carry the CasOS marker: %q", ghcrHostsPath, content)
	}
	// The canonical registry stays as the fallback: a mirror that goes away must
	// cost latency, not the ability to install anything from ghcr.io at all.
	if !strings.Contains(content, `server = "https://ghcr.io"`) {
		t.Fatalf("%s does not fall back to ghcr.io: %q", ghcrHostsPath, content)
	}
	if _, written := runner.written[dockerHubHostsPath]; written {
		t.Fatal("a ghcr.io-only selection wrote the Docker Hub mirror")
	}
}

func TestReconcileRegistryMirrorFileWithoutLegacyContent(t *testing.T) {
	runner := newFakeRegistryMirrorRunner(nil)
	action, err := reconcileRegistryMirrorFile(context.Background(), runner, ghcrHostsPath, GenerateGhcrHostsToml(), "", false)
	if err != nil {
		t.Fatalf("reconcileRegistryMirrorFile: %v", err)
	}
	if action != "absent" {
		t.Fatalf("reconcileRegistryMirrorFile = %q, want absent", action)
	}
	// An empty legacy digest would match any file that happens to be empty; the
	// sentinel is what keeps a target with no history from claiming one.
	for _, command := range runner.commands {
		if strings.Contains(command, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") {
			t.Fatalf("cleanup compared against the digest of empty content: %s", command)
		}
	}
}
