import {useCallback, useEffect, useRef, useState} from "react";
import * as Setting from "@/Setting";

/**
 * Like useResource but tracks page + limit and accepts a fetcher that
 * receives {page, limit}. The server is expected to return either a bare
 * array (legacy endpoints that have not been migrated yet) or
 * {items, total, page, limit}. The hook normalises both shapes so the rest of
 * the page does not have to care which kind of endpoint it is talking to.
 *
 *   const {items, total, page, setPage, limit, setLimit, loading, error, refresh} =
 *     usePaginatedResource(({page, limit}) => Backend.getX(page, limit), [filterKey]);
 *
 * Mounted- and request-id guards come from useResource so a fast page-flapper
 * cannot land an older response on top of a newer one.
 */
export function usePaginatedResource(fetcher, deps = [], options = {}) {
  const {initialData = null, initialPageSize = 20, toastOnError = true} = options;

  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(initialPageSize);
  const [items, setItems] = useState(initialData);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
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

  const load = useCallback(() => {
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setError(null);
    return Promise.resolve()
      .then(() => fetcherRef.current({page, limit}))
      .then((res) => {
        if (!mountedRef.current || requestId !== requestIdRef.current) {
          return;
        }
        if (res && res.status === "ok") {
          const data = res.data;
          if (Array.isArray(data)) {
            // Legacy bare-array response — pretend the whole slice is one page.
            setItems(data);
            setTotal(data.length);
          } else if (data && Array.isArray(data.items)) {
            setItems(data.items);
            setTotal(typeof data.total === "number" ? data.total : data.items.length);
          } else {
            setItems(initialData);
            setTotal(0);
          }
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
    // initialData is read only on the error path; keeping it out of deps
    // avoids re-running when callers pass an inline literal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, limit, toastOnError]);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load, ...deps]);

  const goToPage = useCallback((next) => {
    setPage(next > 0 ? next : 1);
  }, []);

  return {
    items,
    total,
    page,
    limit,
    loading,
    error,
    setPage: goToPage,
    setLimit,
    refresh: load,
  };
}
