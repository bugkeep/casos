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
		if key == "staticBaseUrl" {
			res = "https://cdn.casbin.org"
		} else if key == "logConfig" {
			res = fmt.Sprintf("{\"filename\": \"logs/%s.log\", \"maxdays\":99999, \"perm\":\"0770\"}", GetConfigString("appname"))
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
		dataDir := GetConfigStringDefault("dataDir", defaultDataDir())
		resolvedDataDir = dataDir
		if absolutePath, err := filepath.Abs(dataDir); err == nil {
			resolvedDataDir = absolutePath
		}
		logs.Info("casos data directory: %s", resolvedDataDir)
	})
	return resolvedDataDir
}

// defaultDataDir applies when neither conf/app.conf nor the environment sets
// dataDir, which is the case for a binary running without a config file. The
// per-user directory keeps such a binary on the same state whatever directory
// it was started from; DefaultDataDir is only reached when no per-user
// location can be determined.
func defaultDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "CasOS")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "CasOS")
		}
	default:
		// The XDG base directory specification ignores a relative XDG_DATA_HOME.
		if xdgDataHome := os.Getenv("XDG_DATA_HOME"); strings.HasPrefix(xdgDataHome, "/") {
			return filepath.Join(xdgDataHome, "casos")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "casos")
		}
	}
	return DefaultDataDir
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
