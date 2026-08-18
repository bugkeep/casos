import {Link} from "react-router-dom";
import i18next from "i18next";
import {findLeaf} from "@/nav";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";

/**
 * Derives the trail from the URL against the shared nav tree, so a page never
 * has to declare its own breadcrumb. A path whose first segment is not in the
 * tree renders nothing rather than an invented label.
 */
export function BreadcrumbBar({uri}) {
  const segments = (uri || "").split("/").filter(Boolean);
  if (segments.length === 0) {
    return null;
  }

  const rootLeaf = findLeaf(`/${segments[0]}`);
  if (!rootLeaf) {
    return null;
  }
  const rootLabel = i18next.t(rootLeaf.label);

  const lastSegment = segments[segments.length - 1];
  const lastLeaf = segments.length > 1 ? findLeaf(`/${lastSegment}`) : null;
  const lastLabel = lastLeaf ? i18next.t(lastLeaf.label) : decodeURIComponent(lastSegment);

  return (
    <Breadcrumb>
      <BreadcrumbList className="text-xs sm:gap-1.5">
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link to="/">{i18next.t("general:Home")}</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator />
        {segments.length === 1 ? (
          <BreadcrumbItem>
            <BreadcrumbPage>{rootLabel}</BreadcrumbPage>
          </BreadcrumbItem>
        ) : (
          <>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link to={`/${segments[0]}`}>{rootLabel}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="max-w-[240px] truncate">{lastLabel}</BreadcrumbPage>
            </BreadcrumbItem>
          </>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
