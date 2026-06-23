/* eslint-env jest */
import {buildManagedNodeNameSet, isManagedClusterNode} from "./managedNodeUtils";

test("buildManagedNodeNameSet normalizes managed node names for cluster matching", () => {
  const managedNameSet = buildManagedNodeNameSet([
    {name: "DESKTOP-U0UU4NB"},
    {name: " worker-2 "},
    {name: ""},
    null,
  ]);

  expect(managedNameSet.has("desktop-u0uu4nb")).toBe(true);
  expect(managedNameSet.has("worker-2")).toBe(true);
  expect(managedNameSet.has("DESKTOP-U0UU4NB")).toBe(false);
});

test("isManagedClusterNode matches cluster node names case-insensitively", () => {
  const managedNameSet = new Set(["desktop-u0uu4nb"]);

  expect(isManagedClusterNode("DESKTOP-U0UU4NB", managedNameSet)).toBe(true);
  expect(isManagedClusterNode("desktop-u0uu4nb", managedNameSet)).toBe(true);
  expect(isManagedClusterNode("worker-3", managedNameSet)).toBe(false);
  expect(isManagedClusterNode("", managedNameSet)).toBe(false);
});
