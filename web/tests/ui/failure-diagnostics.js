function stripAnsi(value) {
  return String(value).replace(
    // Covers common CSI/OSC ANSI escape sequences emitted by Playwright.
    // eslint-disable-next-line no-control-regex
    /[\u001b\u009b][[\]()#;?]*(?:\d{1,4}(?:[;:]\d{0,4})*)?[\dA-PR-TZcf-nq-uy=><~]/g,
    "",
  );
}

function createFailureSummary(error, stepName) {
  const errorMessage = stripAnsi(error instanceof Error ? error.message : String(error));
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

module.exports = {
  createFailureSummary,
  stripAnsi,
};
