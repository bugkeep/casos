import test from "node:test";
import assert from "node:assert/strict";
import {getFirstRunChecklist, isFirstRunComplete} from "./firstRunChecklist.js";

test("marks all setup steps from the account and cluster state", () => {
  const steps = getFirstRunChecklist({
    account: {owner: "basic"},
    signinOptions: {signinAvailable: true, autoSignin: false},
    machines: [{name: "worker-1"}],
    releases: [{name: "nginx", namespace: "default"}],
    stats: {nodesTotal: 1, nodesReady: 1, deploymentsTotal: 2},
  });

  assert.deepEqual(steps.map((item) => item.done), [true, true, true, true]);
  assert.equal(isFirstRunComplete(steps), true);
});

test("leaves incomplete steps unchecked and skips password changes for Casdoor", () => {
  const steps = getFirstRunChecklist({
    account: {owner: "basic"},
    signinOptions: {signinAvailable: true, autoSignin: true},
    machines: [],
    releases: [],
    stats: {nodesTotal: 1, nodesReady: 0, deploymentsTotal: 0},
  });

  assert.deepEqual(steps.map((item) => item.done), [false, false, false, false]);
  assert.equal(isFirstRunComplete(steps), false);
  assert.equal(
    getFirstRunChecklist({
      account: {owner: "casdoor"},
      signinOptions: {signinAvailable: false, autoSignin: false},
      machines: [],
      releases: [],
      stats: {nodesTotal: 0, nodesReady: 0, deploymentsTotal: 0},
    })[0].done,
    true
  );
});

test("does not count kube-system workloads as the first installed app", () => {
  // A cluster with no Helm release still reports deployments, because the
  // dashboard counts coredns and anything else running in kube-system.
  const steps = getFirstRunChecklist({
    account: {owner: "basic"},
    signinOptions: {signinAvailable: true, autoSignin: false},
    machines: [{name: "worker-1"}],
    releases: [],
    stats: {nodesTotal: 1, nodesReady: 1, deploymentsTotal: 3},
  });

  assert.equal(steps.at(-1).done, false);
  assert.equal(isFirstRunComplete(steps), false);
});

test("treats an unreachable cluster as unfinished rather than complete", () => {
  // getGlobalMachines, getSigninOptions and getHelmReleases all resolve to null
  // when the request fails, which must not read as "nothing left to do".
  const steps = getFirstRunChecklist({
    account: {owner: "basic"},
    signinOptions: null,
    machines: null,
    releases: null,
    stats: null,
  });

  assert.deepEqual(steps.map((item) => item.done), [false, false, false, false]);
});
