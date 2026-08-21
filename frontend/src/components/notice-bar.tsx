import { CircleAlert, Info, X } from "lucide-react";
import type { Notice } from "@/hooks/use-workspace";
import { cn } from "@/lib/utils";

/** A single-line message above the content: a failed add, or a duplicate notice. */
export function NoticeBar({
  notice,
  onDismiss,
}: {
  notice: Notice;
  onDismiss: () => void;
}) {
  const isError = notice.tone === "error";
  const Icon = isError ? CircleAlert : Info;

  return (
    <div
      role={isError ? "alert" : "status"}
      className={cn(
        "flex items-start gap-2 border-b px-4 py-2 text-sm",
        isError
          ? "border-destructive/30 bg-destructive/10 text-destructive"
          : "border-border bg-muted/50 text-muted-foreground",
      )}
    >
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <p className="min-w-0 flex-1">{notice.message}</p>
      <button
        type="button"
        onClick={onDismiss}
        aria-label="Dismiss"
        className="grid size-5 shrink-0 place-items-center rounded outline-none transition-opacity hover:bg-foreground/10 focus-visible:ring-2 focus-visible:ring-ring/50"
      >
        <X className="size-3.5" aria-hidden="true" />
      </button>
    </div>
  );
}
