import * as React from "react";
import {Eye, EyeOff} from "lucide-react";
import {cn} from "@/lib/utils";
import {Input} from "@/components/ui/input";

/** Password field with a reveal toggle — antd's Input.Password equivalent. */
export function PasswordInput({className, ...props}) {
  const [visible, setVisible] = React.useState(false);
  return (
    <div className="relative">
      <Input type={visible ? "text" : "password"} className={cn("pr-9", className)} {...props} />
      <button
        type="button"
        onClick={() => setVisible((prev) => !prev)}
        className="text-muted-foreground hover:text-foreground absolute top-1/2 right-2 -translate-y-1/2 transition-colors"
        aria-label={visible ? "Hide password" : "Show password"}
        tabIndex={-1}
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}
