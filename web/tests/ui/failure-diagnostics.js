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

function stripAnsi(value) {
  return String(value).replace(
    // Covers common CSI/OSC ANSI escape sequences emitted by Playwright.
    // eslint-disable-next-line no-control-regex
    /[\u001b\u009b][[\]()#;?]*(?:\d{1,4}(?:[;:]\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]/g,
    "",
  );
}

function createFailureSummary(error, stepName) {
  const errorMessage = stripAnsi(getErrorMessage(error));
  const lines = errorMessage
    .split("\n")
    .map((line) => line.trimEnd())
    .filter(Boolean);
  const keyLines = lines.filter((line) => (
    line.startsWith("Error:")
    || line.startsWith("Locator:")
    || line.startsWith("Expected:")
    || line.startsWith("Received:")
    || line.startsWith("Timeout:")
    || line.includes("element(s) not found")
  ));
  const details = (keyLines.length > 0 ? keyLines : lines).slice(0, 6).join("\n");

  return [
    "CI UI test failed",
    `Step: ${stepName}`,
    details,
  ].filter(Boolean).join("\n");
}

function getErrorMessage(error) {
  if (error instanceof Error) {
    return error.message;
  }
  if (error && typeof error.message === "string") {
    return error.message;
  }
  if (error === null || error === undefined) {
    return "Unknown error";
  }
  return String(error);
}

module.exports = {
  createFailureSummary,
  stripAnsi,
};
