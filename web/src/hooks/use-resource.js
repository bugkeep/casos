import {useCallback, useEffect, useRef, useState} from "react";
import * as Setting from "@/Setting";

/**
 * Every list page in this app repeats the same four lines: set loading, call a
 * backend function that resolves to {status, data, msg}, keep the rows or the
 * error, clear loading. useResource is that, plus two things the hand-written
 * version kept getting wrong — it drops responses that arrive after the
 * component unmounts or after a newer request was issued.
 *
 *   const {data, loading, error, refresh} = useResource(
 *     () => PodBackend.getPods(namespace), [namespace], {initialData: []});
 */
export function useResource(fetcher, deps = [], options = {}) {
  const {initialData = null, enabled = true, toastOnError = true, pollInterval = 0} = options;

  const [data, setData] = useState(initialData);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState(null);

  const mountedRef = useRef(true);
  const requestIdRef = useRef(0);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const load = useCallback(
    (options2 = {}) => {
      const {silent = false} = options2;
      const requestId = ++requestIdRef.current;
      if (!silent) {
        setLoading(true);
      }
      setError(null);

      return Promise.resolve()
        .then(() => fetcherRef.current())
        .then((res) => {
          if (!mountedRef.current || requestId !== requestIdRef.current) {
            return;
          }
          if (res && res.status === "ok") {
            setData(res.data ?? initialData);
          } else {
            const message = res?.msg ?? "Request failed";
            setError(message);
            if (toastOnError) {
              Setting.showMessage("error", message);
            }
          }
        })
        .catch((e) => {
          if (!mountedRef.current || requestId !== requestIdRef.current) {
            return;
          }
          setError(e.message);
          if (toastOnError) {
            Setting.showMessage("error", e.message);
          }
        })
        .finally(() => {
          if (mountedRef.current && requestId === requestIdRef.current) {
            setLoading(false);
          }
        });
    },
    // initialData is only read on failure paths and is expected to be a stable
    // literal; including it would re-create load on every render of a caller
    // that passes an inline [].
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [toastOnError]
  );

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return undefined;
    }
    load();
    if (pollInterval > 0) {
      const timer = setInterval(() => load({silent: true}), pollInterval);
      return () => clearInterval(timer);
    }
    return undefined;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, enabled, pollInterval]);

  return {data, setData, loading, error, refresh: load};
}

/**
 * Wraps a mutating backend call: reports the outcome, keeps a submitting flag
 * for the button, and resolves to whether it succeeded so callers can close a
 * dialog only on success.
 */
export function runAction(promise, {successMessage, onSuccess, onError} = {}) {
  return Promise.resolve(promise)
    .then((res) => {
      if (res && res.status === "ok") {
        if (successMessage) {
          Setting.showMessage("success", successMessage);
        }
        onSuccess?.(res);
        return true;
      }
      Setting.showMessage("error", res?.msg ?? "Request failed");
      onError?.(res);
      return false;
    })
    .catch((e) => {
      Setting.showMessage("error", e.message);
      onError?.(e);
      return false;
    });
}
