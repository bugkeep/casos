/* eslint-env jest */
import {
  getManagedNodesTableScrollX,
  managedNodeActionsColumnFixed,
  managedNodeActionsColumnWidth
} from "./managedNodeTableLayout";

test("managed nodes table enables horizontal scroll for action buttons", () => {
  expect(getManagedNodesTableScrollX()).toBeGreaterThanOrEqual(1500);
});

test("managed nodes actions column reserves enough width for three buttons", () => {
  expect(managedNodeActionsColumnWidth).toBeGreaterThanOrEqual(280);
});

test("managed nodes actions stay pinned to the right edge", () => {
  expect(managedNodeActionsColumnFixed).toBe("right");
});
