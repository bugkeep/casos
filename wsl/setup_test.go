package wsl

import (
	"io"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, writer io.Writer, text string) {
	t.Helper()
	if _, err := io.WriteString(writer, text); err != nil {
		t.Fatalf("write %q: %v", text, err)
	}
}

func TestParseDistroList(t *testing.T) {
	stdout := strings.Join([]string{
		"  NAME                   STATE           VERSION",
		"* Ubuntu                 Running         2",
		"  openSUSE Leap 15       Stopped         2",
		"  docker-desktop         Running         2",
		"  Legacy                 Stopped         1",
		"",
	}, "\r\n")

	distros := parseDistroList(stdout)
	if len(distros) != 4 {
		t.Fatalf("parsed %d distros, want 4: %+v", len(distros), distros)
	}
	if distros[0].Name != "Ubuntu" || !distros[0].Default || distros[0].Version != 2 || distros[0].State != "Running" {
		t.Errorf("unexpected default distro: %+v", distros[0])
	}
	if distros[1].Name != "openSUSE Leap 15" {
		t.Errorf("name with spaces = %q", distros[1].Name)
	}
	if distros[1].Default {
		t.Error("only the line starting with * is the default distro")
	}
	if distros[2].Usable() {
		t.Error("docker-desktop cannot host a worker node")
	}
	if distros[3].Usable() {
		t.Error("a WSL 1 distro cannot run systemd")
	}
}

// The header row is localized, so it must be recognised by shape rather than by
// its text: it is the row that does not end in a WSL version number.
func TestParseDistroListLocalizedHeader(t *testing.T) {
	stdout := "  NAME            STATE           VERSION\n* Ubuntu          已停止          2\n"
	distros := parseDistroList(stdout)
	if len(distros) != 1 || distros[0].Name != "Ubuntu" {
		t.Fatalf("parsed %+v, want only Ubuntu", distros)
	}
}

func TestParseDistroListNoDistros(t *testing.T) {
	stdout := "Windows Subsystem for Linux has no installed distributions.\n"
	if distros := parseDistroList(stdout); len(distros) != 0 {
		t.Errorf("parsed %+v, want none", distros)
	}
}

func TestNodeDistro(t *testing.T) {
	cases := map[string]struct {
		distros []Distro
		want    string
	}{
		"prefers the default distro": {
			distros: []Distro{
				{Name: "Debian", Version: 2},
				{Name: "Ubuntu", Version: 2, Default: true},
			},
			want: "Ubuntu",
		},
		"skips an unusable default": {
			distros: []Distro{
				{Name: "docker-desktop", Version: 2, Default: true},
				{Name: "Ubuntu", Version: 2},
			},
			want: "Ubuntu",
		},
		"reports none when nothing is usable": {
			distros: []Distro{{Name: "Legacy", Version: 1}},
			want:    "",
		},
		"reports none without distros": {want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			selected := (&Status{Distros: tc.distros}).NodeDistro()
			got := ""
			if selected != nil {
				got = selected.Name
			}
			if got != tc.want {
				t.Errorf("NodeDistro = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestForwardProgressSplitsRedrawsAndHoldsPartialLines(t *testing.T) {
	lines := []string{}
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		forwardProgress(reader, func(line string) { lines = append(lines, line) })
		close(done)
	}()

	mustWrite(t, writer, "Installing: Ubuntu\r 10%\r 10%\r100%\r\nInstall")
	mustWrite(t, writer, "ed\n")
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	<-done

	want := []string{"Installing: Ubuntu", "10%", "100%", "Installed"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}
