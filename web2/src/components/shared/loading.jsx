import {cn} from "@/lib/utils";

const DOT_COUNT = 3;

/**
 * The bouncing-dot indicator the old UI used in place of a spinner. Keeping it
 * means a page that swaps from antd to shadcn does not also change what "busy"
 * looks like to someone watching a cluster come up.
 */
export function AiDots({size = "medium", className}) {
  const dotClass = {
    small: "size-1",
    medium: "size-2",
    large: "size-3",
  }[size];
  const gapClass = {small: "gap-1", medium: "gap-1.5", large: "gap-2.5"}[size];

  return (
    <span className={cn("inline-flex items-center", gapClass, className)}>
      {Array.from({length: DOT_COUNT}).map((_, index) => (
        <span
          key={index}
          className={cn("bg-foreground inline-block rounded-full", dotClass)}
          style={{animation: "casos-dot-bounce 1.4s ease-in-out infinite", animationDelay: `${index * 0.16}s`}}
        />
      ))}
    </span>
  );
}

export function Loading({spinning = true, tip, type = "section", className}) {
  if (!spinning) {
    return null;
  }
  const isPage = type === "page";
  const isSmall = type === "small";

  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center",
        isPage && "h-[calc(100vh-160px)] w-full",
        type === "section" && "py-12",
        className
      )}
    >
      <AiDots size={isSmall ? "small" : isPage ? "large" : "medium"} />
      {tip && !isSmall ? <div className="text-muted-foreground mt-3.5 text-xs tracking-wide">{tip}</div> : null}
    </div>
  );
}

export default Loading;
