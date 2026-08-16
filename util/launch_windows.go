// Copyright 2026 The Casos Authors. All Rights Reserved.
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

package util

import (
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// OpenBrowser opens url in the user's default browser.
//
// rundll32 is used rather than "cmd /c start" so that url is delivered as one
// argument and never reaches a command interpreter, and it is resolved against
// the real system directory so a rundll32.exe planted earlier on PATH cannot be
// picked up instead.
func OpenBrowser(url string) error {
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}

	cmd := exec.Command(filepath.Join(systemDirectory, "rundll32.exe"), "url.dll,FileProtocolHandler", url)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

// ReportStartupFailure shows message in a modal dialog.
//
// A double-clicked CasOS logs to a console that the in-process Kubernetes
// components flood within seconds, so a startup failure printed there is not
// something the user can be expected to notice — and a user who never opened a
// terminal is exactly the one who needs to be told that CasOS did not come up.
func ReportStartupFailure(message string) {
	caption, err := windows.UTF16PtrFromString("CasOS")
	if err != nil {
		return
	}

	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}

	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
