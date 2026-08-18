import {cn} from "@/lib/utils";

export function PageContainer({className, children}) {
  return <div className={cn("flex flex-col gap-4 p-4 md:p-6", className)}>{children}</div>;
}

export function PageHeader({title, description, actions, className}) {
  return (
    <div className={cn("flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between", className)}>
      <div className="min-w-0">
        <h1 className="truncate text-xl font-semibold tracking-tight">{title}</h1>
        {description ? <p className="text-muted-foreground mt-1 text-sm">{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  );
}
