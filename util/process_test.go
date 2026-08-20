package util

import "testing"

func TestParseProcessImageName(t *testing.T) {
	tests := []struct {
		name   string
		output string
		goos   string
		want   string
	}{
		{
			name:   "tasklist CSV record",
			output: "\"casos.exe\",\"4242\",\"Console\",\"1\",\"52,140 K\"\r\n",
			goos:   "windows",
			want:   "casos.exe",
		},
		{
			name:   "tasklist reports no match",
			output: "INFO: No tasks are running which match the specified criteria.\r\n",
			goos:   "windows",
			want:   "",
		},
		{
			name:   "ps command name",
			output: "casos\n",
			goos:   "linux",
			want:   "casos",
		},
		{
			name:   "ps prints a path",
			output: "/usr/local/bin/casos\n",
			goos:   "darwin",
			want:   "casos",
		},
		{
			name:   "no output at all",
			output: "   \n",
			goos:   "linux",
			want:   "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseProcessImageName(test.output, test.goos); got != test.want {
				t.Errorf("parseProcessImageName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsOwnExecutableRejectsAnUnknownProcess(t *testing.T) {
	// PID 0 is never a real process to look up on any supported platform.
	if isOwnExecutable(0) {
		t.Error("isOwnExecutable(0) = true, want false for a process that cannot be identified")
	}
}
