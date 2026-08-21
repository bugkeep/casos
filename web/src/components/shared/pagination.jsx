import React from "react";
import {ChevronLeft, ChevronRight} from "lucide-react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";

/**
 * Server-side pager. The list page owns page + limit and the total count of
 * matching items; this component only renders the controls. Pages outside
 * [1, totalPages] are clamped silently by the caller, so the buttons here can
 * trust the page numbers they receive.
 *
 *   <Pagination page={page} limit={limit} total={total} onPageChange={setPage} />
 */
export function Pagination({page, limit, total, onPageChange, className}) {
  const totalPages = Math.max(1, Math.ceil(total / limit));
  const canPrev = page > 1;
  const canNext = page < totalPages;
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);

  return (
    <div
      data-slot="pagination"
      className={cn("flex items-center justify-between border-t px-4 py-2.5", className)}
    >
      <span className="text-muted-foreground text-xs">
        {start}–{end} of {total}
      </span>
      <div className="flex items-center gap-1">
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => onPageChange(page - 1)}
          disabled={!canPrev}
          aria-label="Previous page"
          data-slot="pagination-prev"
        >
          <ChevronLeft className="size-4" />
        </Button>
        <span className="text-muted-foreground px-2 text-xs tabular-nums">
          {page} / {totalPages}
        </span>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => onPageChange(page + 1)}
          disabled={!canNext}
          aria-label="Next page"
          data-slot="pagination-next"
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  );
}
