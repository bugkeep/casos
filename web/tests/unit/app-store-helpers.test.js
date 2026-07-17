const assert = require("node:assert/strict");
const test = require("node:test");
const {replaceInstallValuesIfProvided} = require("../ui/app-store-helpers");

test("preserves App Store install values when no override is provided", async() => {
  const filled = [];
  const textarea = {fill: async value => filled.push(value)};

  await replaceInstallValuesIfProvided(textarea, undefined);

  assert.deepEqual(filled, []);
});

test("replaces App Store install values when an override is provided", async() => {
  const filled = [];
  const textarea = {fill: async value => filled.push(value)};

  await replaceInstallValuesIfProvided(textarea, "service:\n  type: NodePort\n");

  assert.deepEqual(filled, ["service:\n  type: NodePort\n"]);
});
