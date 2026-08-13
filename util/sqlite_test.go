package util

import (
	"runtime"
	"testing"
)

func TestSQLiteDatabasePath(t *testing.T) {
	tests := []struct {
		name           string
		dataSourceName string
		want           string
	}{
		{
			name:           "bare path",
			dataSourceName: "/var/lib/casos/casos.db",
			want:           "/var/lib/casos/casos.db",
		},
		{
			name:           "query string is stripped",
			dataSourceName: "/var/lib/casos/casos.db?_pragma=journal_mode(WAL)&_txlock=immediate",
			want:           "/var/lib/casos/casos.db",
		},
		{
			name:           "file URI",
			dataSourceName: "file:/var/lib/casos/casos.db?cache=shared",
			want:           "/var/lib/casos/casos.db",
		},
		{
			name:           "in-memory database has no path",
			dataSourceName: ":memory:",
			want:           "",
		},
		{
			name:           "empty data source",
			dataSourceName: "",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SQLiteDatabasePath(tt.dataSourceName)
			if err != nil {
				t.Fatalf("SQLiteDatabasePath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("SQLiteDatabasePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSQLiteDatabasePathWindowsDriveLetter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive letters are only stripped on Windows")
	}

	for _, dataSourceName := range []string{
		"/C:/casos/data/kine/state.db",
		"file:/C:/casos/data/kine/state.db?cache=shared",
	} {
		got, err := SQLiteDatabasePath(dataSourceName)
		if err != nil {
			t.Fatalf("SQLiteDatabasePath(%q) error = %v", dataSourceName, err)
		}
		if want := "C:/casos/data/kine/state.db"; got != want {
			t.Fatalf("SQLiteDatabasePath(%q) = %q, want %q", dataSourceName, got, want)
		}
	}
}

func TestEnsureSQLiteDirectoryIgnoresInMemory(t *testing.T) {
	if err := EnsureSQLiteDirectory(":memory:"); err != nil {
		t.Fatalf("EnsureSQLiteDirectory() error = %v", err)
	}
}
