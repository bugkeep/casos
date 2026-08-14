// Copyright 2023 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package conf

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
)

const DefaultDataDir = "./data"

var builtInDefaults = map[string]string{
	"appname":             "casos",
	"driverName":          "sqlite",
	"casdoorEndpoint":     "https://door.casdoor.com",
	"clientId":            "af6b5aa958822fb9dc33",
	"clientSecret":        "8bc3010c1c951c8d876b1f311a901ff8deeb93bc",
	"casdoorOrganization": "casbin",
	"casdoorApplication":  "app-casibase",
}

var (
	dataDirOnce     sync.Once
	resolvedDataDir string
)

func init() {
	// this array contains the beego configuration items that may be modified via env
	presetConfigItems := []string{"httpport", "appname"}
	for _, key := range presetConfigItems {
		if value, ok := os.LookupEnv(key); ok {
			err := beego.AppConfig.Set(key, value)
			if err != nil {
				panic(err)
			}
		}
	}
}

func GetConfigString(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	// the only place in the codebase that reads beego's app.conf directly
	res := beego.AppConfig.String(key)
	if res == "" {
		res = standaloneConfigValue(key)
	}
	if res == "" {
		if value, ok := builtInDefaults[key]; ok {
			res = value
		} else if key == "staticBaseUrl" {
			res = "https://cdn.casbin.org"
		} else if key == "logConfig" {
			res = fmt.Sprintf("{\"filename\": %q, \"maxdays\":99999, \"perm\":\"0600\"}", filepath.Join(GetDataDir(), "logs", GetConfigString("appname")+".log"))
		}
	}

	return res
}

// GetConfigStringDefault returns the value for key, falling back to defaultValue
// when the key is unset or blank.
func GetConfigStringDefault(key string, defaultValue string) string {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func GetConfigBool(key string) bool {
	return GetConfigBoolDefault(key, false)
}

// GetConfigBoolDefault parses key as a boolean, falling back to defaultValue when
// the key is unset or cannot be parsed. Accepts the strconv forms plus
// yes/y/on and no/n/off, case-insensitively.
func GetConfigBoolDefault(key string, defaultValue bool) bool {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}

	switch strings.ToLower(value) {
	case "yes", "y", "on":
		return true
	case "no", "n", "off":
		return false
	}

	res, err := strconv.ParseBool(value)
	if err != nil {
		logs.Warning("invalid boolean config %s=%q, using default %t", key, value, defaultValue)
		return defaultValue
	}
	return res
}

func GetConfigInt(key string) int {
	value := GetConfigString(key)
	num, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return num
}

// GetConfigIntDefault parses key as an int, falling back to defaultValue when the
// key is unset or cannot be parsed.
func GetConfigIntDefault(key string, defaultValue int) int {
	value := strings.TrimSpace(GetConfigString(key))
	if value == "" {
		return defaultValue
	}

	res, err := strconv.Atoi(value)
	if err != nil {
		logs.Warning("invalid int config %s=%q, using default %d", key, value, defaultValue)
		return defaultValue
	}
	return res
}

// GetHTTPListenAddress preserves the all-interface source-build listener while
// defaulting standalone binaries to loopback. httpBind overrides either default.
func GetHTTPListenAddress(port int) string {
	bind := strings.TrimSpace(GetConfigString("httpBind"))
	if bind == "" && standaloneBuild {
		bind = "127.0.0.1"
	}
	return net.JoinHostPort(bind, strconv.Itoa(port))
}

func IsStandaloneBuild() bool {
	return standaloneBuild
}

func GetConfigInt64(key string) (int64, error) {
	value := GetConfigString(key)
	num, err := strconv.ParseInt(value, 10, 64)
	return num, err
}

func GetConfigDataSourceName() string {
	dataSourceName := GetConfigString("dataSourceName")

	runningInDocker := os.Getenv("RUNNING_IN_DOCKER")
	if runningInDocker == "true" {
		// https://stackoverflow.com/questions/48546124/what-is-linux-equivalent-of-host-docker-internal
		if runtime.GOOS == "linux" {
			dataSourceName = strings.ReplaceAll(dataSourceName, "localhost", "172.17.0.1")
		} else {
			dataSourceName = strings.ReplaceAll(dataSourceName, "localhost", "host.docker.internal")
		}
	}

	return dataSourceName
}

func GetDatabaseDriverName() string {
	driverName := strings.ToLower(GetConfigStringDefault("driverName", "sqlite"))
	if driverName == "sqlite3" {
		return "sqlite"
	}
	return driverName
}

func GetDatabaseDataSourceName() string {
	return resolveDatabaseDataSourceName(
		GetDatabaseDriverName(),
		GetConfigDataSourceName(),
		GetDataDir(),
	)
}

// GetDataDir returns the absolute data directory. A relative dataDir is
// resolved against the working directory once per process: resolving it on
// every call would silently move the SQLite databases and the node credential
// key if anything ever changed the working directory.
func GetDataDir() string {
	dataDirOnce.Do(func() {
		userDataDir := ""
		if standaloneBuild {
			userDataDir = defaultUserDataDir()
		}
		configuredDataDir := strings.TrimSpace(os.Getenv("dataDir"))
		if configuredDataDir == "" {
			configuredDataDir = strings.TrimSpace(beego.AppConfig.String("dataDir"))
		}
		dataDir := resolveDataDir(
			standaloneBuild,
			os.Getenv("CASOS_DATA_DIR"),
			configuredDataDir,
			userDataDir,
		)
		resolvedDataDir = dataDir
		if absolutePath, err := filepath.Abs(dataDir); err == nil {
			resolvedDataDir = absolutePath
		}
		logs.Info("casos data directory: %s", resolvedDataDir)
	})
	return resolvedDataDir
}

func resolveDataDir(standalone bool, casosDataDir, configuredDataDir, userDataDir string) string {
	if dataDir := strings.TrimSpace(casosDataDir); dataDir != "" {
		return dataDir
	}
	if dataDir := strings.TrimSpace(configuredDataDir); dataDir != "" {
		return dataDir
	}
	if standalone {
		return userDataDir
	}
	return DefaultDataDir
}

func defaultUserDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return defaultDataDir(runtime.GOOS, home, os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"))
}

func defaultDataDir(goos, home, xdgDataHome, localAppData string) string {
	switch goos {
	case "windows":
		if localAppData != "" {
			return filepath.Join(localAppData, "CasOS")
		}
	case "darwin":
		if home != "" {
			return filepath.Join(home, "Library", "Application Support", "CasOS")
		}
	default:
		if xdgDataHome != "" {
			return filepath.Join(xdgDataHome, "casos")
		}
		if home != "" {
			return filepath.Join(home, ".local", "share", "casos")
		}
	}
	return DefaultDataDir
}

func EnsureDataDir() error {
	dataDir := GetDataDir()
	for _, dir := range []string{dataDir, filepath.Join(dataDir, "logs"), GetSessionDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create data directory %q: %w", dir, err)
		}
	}
	return nil
}

func GetSessionDir() string {
	configuredDataDir := strings.TrimSpace(os.Getenv("dataDir"))
	if configuredDataDir == "" {
		configuredDataDir = strings.TrimSpace(beego.AppConfig.String("dataDir"))
	}
	return resolveSessionDir(standaloneBuild, os.Getenv("CASOS_DATA_DIR"), configuredDataDir, GetDataDir())
}

func resolveSessionDir(standalone bool, casosDataDir, configuredDataDir, dataDir string) string {
	if strings.TrimSpace(casosDataDir) == "" && (!standalone || strings.TrimSpace(configuredDataDir) != "") {
		return "./tmp"
	}
	return filepath.Join(dataDir, "sessions")
}

func resolveDatabaseDataSourceName(driverName, dataSourceName, dataDir string) string {
	if driverName == "sqlite" && strings.TrimSpace(dataSourceName) == "" {
		return filepath.Join(dataDir, "casos.db")
	}
	return dataSourceName
}

func GetLanguage(language string) string {
	if language == "" || language == "*" {
		return "en"
	}

	if len(language) != 2 || language == "nu" {
		return "en"
	} else {
		return language
	}
}

func IsDemoMode() bool {
	return GetConfigBoolDefault("isDemoMode", false)
}

func GetConfigBatchSize() int {
	res, err := strconv.Atoi(GetConfigString("batchSize"))
	if err != nil {
		res = 100
	}
	return res
}
