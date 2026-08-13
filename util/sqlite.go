package util

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SQLiteDatabasePath extracts the file system path from a SQLite data source
// name. It accepts a bare path as well as a "file:" URI, with or without a
// query string. An empty path is returned for in-memory databases, which have
// no directory to create.
func SQLiteDatabasePath(dataSourceName string) (string, error) {
	databasePath := strings.SplitN(dataSourceName, "?", 2)[0]
	if strings.HasPrefix(databasePath, "file:") {
		uri, err := url.Parse(databasePath)
		if err != nil {
			return "", fmt.Errorf("parse SQLite data source: %w", err)
		}
		databasePath = uri.Path
		if uri.Host != "" {
			return "//" + uri.Host + databasePath, nil
		}
	}
	// A Windows path carried in a URI or a slash-separated endpoint arrives as
	// "/C:/dir/state.db"; the leading separator is not part of the path.
	if runtime.GOOS == "windows" && len(databasePath) >= 3 && databasePath[0] == '/' && databasePath[2] == ':' {
		databasePath = databasePath[1:]
	}
	if databasePath == ":memory:" {
		return "", nil
	}
	return databasePath, nil
}

// EnsureSQLiteDirectory creates the parent directory of a SQLite database so
// the driver can create the database file on first use.
func EnsureSQLiteDirectory(dataSourceName string) error {
	databasePath, err := SQLiteDatabasePath(dataSourceName)
	if err != nil {
		return err
	}
	if databasePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return fmt.Errorf("create SQLite database directory: %w", err)
	}
	return nil
}
