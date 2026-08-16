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

//go:build !windows

package util

// OpenBrowser is a no-op off Windows. StartedByDoubleClick is false on every
// other platform, so nothing reaches this, and CasOS on Linux is routinely
// started by a service manager that has no desktop session to open a browser
// into.
func OpenBrowser(string) error {
	return nil
}

// ReportStartupFailure is a no-op off Windows, where a startup failure is
// already visible on the terminal or in the service manager's journal.
func ReportStartupFailure(string) {}
