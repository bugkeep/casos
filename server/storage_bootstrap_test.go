package server

import (
	"strings"
	"testing"
)

func TestLocalPathRootDirIsIndependentOfTheHostDataDir(t *testing.T) {
	// The provisioner runs on the node, so a Windows dataDir on the CasOS host
	// must not reach it — and must not disqualify it either, which is what the
	// earlier filepath-versus-path mismatch did.
	cfg := Config{
		DataDir:          `D:\github_repos\casos\data`,
		LocalPathRootDir: defaultLocalPathRootDir,
	}

	got, err := localPathRootDir(cfg)
	if err != nil {
		t.Fatalf("localPathRootDir() error = %v, want a usable node path", err)
	}
	if got != defaultLocalPathRootDir {
		t.Fatalf("localPathRootDir() = %q, want %q", got, defaultLocalPathRootDir)
	}
}

func TestLocalPathRootDirRejectsUnusablePaths(t *testing.T) {
	tests := []struct {
		name    string
		rootDir string
	}{
		{"empty", "  "},
		{"relative", "var/lib/casos"},
		{"host path from a Windows dataDir", `D:\github_repos\casos\data\local-path-provisioner`},
		{"filesystem root", "/"},
		{"one level below root", "/mnt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := localPathRootDir(Config{LocalPathRootDir: test.rootDir}); err == nil {
				t.Fatalf("localPathRootDir(%q) succeeded, want an error", test.rootDir)
			}
		})
	}
}

func TestLocalPathRootDirTrimsTrailingSeparator(t *testing.T) {
	got, err := localPathRootDir(Config{LocalPathRootDir: "/var/lib/casos/local-path-provisioner/"})
	if err != nil {
		t.Fatalf("localPathRootDir() error = %v", err)
	}
	if got != defaultLocalPathRootDir {
		t.Fatalf("localPathRootDir() = %q, want %q", got, defaultLocalPathRootDir)
	}
	// A trailing separator would otherwise reach the teardown script's prefix
	// check as a doubled slash that no volume path matches.
	if strings.HasSuffix(got, "/") {
		t.Fatalf("localPathRootDir() = %q, want no trailing separator", got)
	}
}
