import React, {useEffect, useRef, useState} from "react";
import {BadgeCheck, Boxes, Search, Star} from "lucide-react";
import * as PodBackend from "@/backend/PodBackend";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {Loading} from "@/components/shared/loading";

const SEARCH_DEBOUNCE = 400;

function formatPullCount(count) {
  if (count >= 1e9) {
    return `${(count / 1e9).toFixed(1)}B`;
  }
  if (count >= 1e6) {
    return `${(count / 1e6).toFixed(1)}M`;
  }
  if (count >= 1e3) {
    return `${(count / 1e3).toFixed(0)}K`;
  }
  return String(count ?? 0);
}

// Docker Hub logo URLs 404 often enough that a broken-image icon would be the
// common case; the initial-letter tile is the fallback.
function ImageLogo({name, logoUrl}) {
  const [failed, setFailed] = useState(false);

  if (!logoUrl || failed) {
    return (
      <span className="bg-info/10 text-info flex size-7 shrink-0 items-center justify-center rounded-md font-mono text-xs font-bold">
        {name ? name[0].toUpperCase() : "?"}
      </span>
    );
  }

  return (
    <img
      src={logoUrl}
      alt={name}
      width={28}
      height={28}
      onError={() => setFailed(true)}
      className="bg-muted size-7 shrink-0 rounded-md object-contain"
    />
  );
}

/** Image picker backed by the Docker Hub search API. */
export function DockerHubDialog({open, onCancel, onSelect}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const timerRef = useRef(null);

  function search(term) {
    if (!term.trim()) {
      setResults([]);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    PodBackend.searchDockerHubImages(term)
      .then((res) => {
        if (res.status === "ok") {
          setResults(res.data ?? []);
        } else {
          setError(res.msg);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    if (!open) {
      return;
    }
    // Opening with a populated grid reads better than an empty pane, and nginx
    // is the image people most often reach for first.
    setQuery("");
    setError(null);
    search("nginx");
  }, [open]);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  function handleQueryChange(next) {
    setQuery(next);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => search(next), SEARCH_DEBOUNCE);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? null : onCancel())}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Boxes className="text-info size-4" />
            Docker Hub
          </DialogTitle>
        </DialogHeader>

        <div className="relative">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(event) => handleQueryChange(event.target.value)}
            placeholder="Search Docker Hub images…"
            className="pl-9"
            autoFocus
          />
        </div>

        {error ? <MessageAlert title={error} /> : null}

        <div className="scrollbar-thin max-h-[55vh] overflow-y-auto pr-1">
          {loading && results.length === 0 ? (
            <Loading />
          ) : (
            <div className="grid gap-2 sm:grid-cols-2">
              {results.map((image) => (
                <button
                  key={image.name}
                  type="button"
                  onClick={() => onSelect(`${image.name}:latest`)}
                  className="hover:border-ring/60 hover:bg-accent/40 grid gap-1 rounded-lg border p-3 text-left transition-colors"
                >
                  <div className="flex items-center gap-2">
                    <ImageLogo name={image.name} logoUrl={image.logoUrl} />
                    <span className="min-w-0 flex-1 truncate text-sm font-semibold">{image.name}</span>
                    {image.isOfficial ? (
                      <SimpleTooltip title="Official Image">
                        <BadgeCheck className="text-info size-4 shrink-0" />
                      </SimpleTooltip>
                    ) : null}
                  </div>
                  {image.description ? (
                    <p className="text-muted-foreground line-clamp-2 text-xs leading-relaxed">{image.description}</p>
                  ) : null}
                  <div className="text-muted-foreground flex gap-3 text-xs">
                    <span className="flex items-center gap-1">
                      <Star className="size-3" />
                      {image.starCount?.toLocaleString() ?? 0}
                    </span>
                    <span>↓ {formatPullCount(image.pullCount)}</span>
                  </div>
                </button>
              ))}

              {!loading && results.length === 0 && query ? (
                <p className="text-muted-foreground col-span-full py-10 text-center text-sm">
                  No results for &quot;{query}&quot;
                </p>
              ) : null}
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export default DockerHubDialog;
