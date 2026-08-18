// The four things a fresh install needs before CasOS can run anything. Every
// step is derived from server state rather than a local "seen it" flag, so the
// checklist stays correct after a refresh or a sign-in from another browser.
export function getFirstRunChecklist({account, signinOptions, machines, releases, stats}) {
  // The server reports autoSignin only while the built-in admin still has the
  // default password, which makes it the one honest signal that it changed.
  const passwordDone = Boolean(
    account &&
      (account.owner !== "basic" ||
        signinOptions?.signinAvailable === false ||
        signinOptions?.autoSignin === false)
  );

  return [
    {key: "password", done: passwordDone},
    {key: "machine", done: Array.isArray(machines) && machines.length > 0},
    {key: "node", done: Number(stats?.nodesReady) > 0},
    // Not the dashboard's deployment count: that one spans every namespace, so
    // kube-system's own coredns keeps it above zero on a cluster where nothing
    // has been installed. App Store installs are Helm releases, and only those.
    {key: "app", done: Array.isArray(releases) && releases.length > 0},
  ];
}

export function isFirstRunComplete(steps) {
  return Array.isArray(steps) && steps.length > 0 && steps.every((step) => step.done);
}

const firstRunChecklistDoneKey = "casos.firstRunChecklistDone";

// Setup only happens once, so the completed checklist is remembered and the
// three requests behind it stop being issued on every later dashboard visit.
// A cluster rebuilt from scratch keeps the flag; clearing site data resets it.
export function readFirstRunChecklistDone() {
  try {
    return window.localStorage.getItem(firstRunChecklistDoneKey) === "true";
  } catch (_) {
    // Storage may be unavailable; the checklist then re-derives itself instead.
    return false;
  }
}

export function markFirstRunChecklistDone() {
  try {
    window.localStorage.setItem(firstRunChecklistDoneKey, "true");
  } catch (_) {
    // Same as above: losing the flag only costs one more round of requests.
  }
}
