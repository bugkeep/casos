import React from "react";
import {X} from "lucide-react";
import {Alert, AlertDescription, AlertTitle} from "@/components/ui/alert";
import {Button} from "@/components/ui/button";
import {CodeText} from "@/components/shared/misc";

/**
 * A Helm chart that cannot install because the cluster lacks an API the chart
 * requires. The backend sometimes returns that as a structured object naming the
 * missing GroupVersionKind — surfacing it is the difference between "install
 * failed" and knowing which CRD to install.
 */
export function HelmCompatibilityErrorAlert({error, onClose, t}) {
  if (!error) {
    return null;
  }
  const structured = typeof error === "object";
  const hasDetail = structured && (error.gvk || error.detail);

  return (
    <Alert variant="destructive" className="relative">
      <AlertTitle className="pr-8">{structured ? error.message : error}</AlertTitle>
      {hasDetail ? (
        <AlertDescription>
          {error.gvk ? (
            <div>
              <span className="font-medium">{t("helm:Missing resource")}</span>
              <br />
              <CodeText>{error.gvk}</CodeText>
            </div>
          ) : null}
          {error.detail ? (
            <div className={error.gvk ? "mt-2" : undefined}>
              <span className="font-medium">{t("helm:Compatibility details")}</span>
              <br />
              {error.detail}
            </div>
          ) : null}
        </AlertDescription>
      ) : null}
      {onClose ? (
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={onClose}
          className="absolute top-2 right-2"
          aria-label="Dismiss"
        >
          <X className="size-3.5" />
        </Button>
      ) : null}
    </Alert>
  );
}

export default HelmCompatibilityErrorAlert;
