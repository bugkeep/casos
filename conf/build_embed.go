//go:build embed

package conf

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
)

// A standalone binary ships as a single executable with no conf directory
// beside it, so conf/app.conf is compiled in.
//
//go:embed app.conf
var embeddedAppConfig []byte

var embeddedConfigLoaded bool

// IsEmbeddedConfig reports whether CasOS is running on the compiled-in
// conf/app.conf because no copy was found on disk. The compiled-in file
// describes a source checkout, so a few of its settings have to be
// reinterpreted for an installed binary.
func IsEmbeddedConfig() bool {
	return embeddedConfigLoaded
}

// loadEmbeddedConfig points beego at the compiled-in conf/app.conf when no copy
// exists in the working directory or next to the executable. An on-disk file
// always wins, so dropping a conf/app.conf beside the binary remains the way to
// configure a standalone installation.
//
// Replacing beego's config wholesale, rather than answering individual keys,
// keeps a single configuration source at runtime: every reader — including the
// ones that go straight to beego.AppConfig — sees the same values, and no
// lookup can silently take precedence over a default declared in Go.
//
// beego only loads a config from a path, so the embedded bytes take a detour
// through a temporary file. beego parses it eagerly, which is why the file is
// removed again as soon as it has been read.
func loadEmbeddedConfig() {
	for _, candidate := range configSearchPaths() {
		if _, err := os.Stat(candidate); err == nil {
			return
		}
	}

	tempDir, err := os.MkdirTemp("", "casos-conf-*")
	if err != nil {
		logs.Warning("embedded config: create temporary directory: %v", err)
		return
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "app.conf")
	if err := os.WriteFile(configPath, embeddedAppConfig, 0o600); err != nil {
		logs.Warning("embedded config: write %q: %v", configPath, err)
		return
	}
	if err := beego.LoadAppConfig("ini", configPath); err != nil {
		logs.Warning("embedded config: load %q: %v", configPath, err)
		return
	}

	embeddedConfigLoaded = true
	logs.Info("casos configuration: compiled-in conf/app.conf")
}

// configSearchPaths mirrors beego's own lookup for conf/app.conf, so the
// compiled-in copy is used exactly when beego found nothing to load.
func configSearchPaths() []string {
	var paths []string
	if workingDir, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(workingDir, "conf", "app.conf"))
	}
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "conf", "app.conf"))
	}
	return paths
}
