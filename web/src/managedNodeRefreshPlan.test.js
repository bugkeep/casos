/* eslint-env jest */
import {
  shouldPollClusterNodesInBackground,
  shouldRefreshClusterNodesAfterManagedDeploy,
  shouldRefreshClusterNodesAfterManagedRemove,
  shouldRefreshClusterNodesAfterManagedRepair
} from "./managedNodeRefreshPlan";

test("managed deploy refreshes cluster nodes", () => {
  expect(shouldRefreshClusterNodesAfterManagedDeploy()).toBe(true);
});

test("managed repair refreshes cluster nodes", () => {
  expect(shouldRefreshClusterNodesAfterManagedRepair()).toBe(true);
});

test("managed remove refreshes cluster nodes", () => {
  expect(shouldRefreshClusterNodesAfterManagedRemove()).toBe(true);
});

test("nodes page polls cluster nodes in the background", () => {
  expect(shouldPollClusterNodesInBackground()).toBe(true);
});
