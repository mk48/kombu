import { GitFork, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";

/** Shown when no repositories have been added yet. */
export function EmptyWorkspace({
  onAdd,
  picking,
}: {
  onAdd: () => void;
  picking: boolean;
}) {
  return (
    <div className="flex flex-1 items-center justify-center p-8">
      <div className="flex max-w-sm flex-col items-center gap-5 text-center">
        <GitFork className="size-10 text-muted-foreground/40" aria-hidden="true" />
        <div className="space-y-1.5">
          <h1 className="text-lg font-semibold tracking-tight">
            No repositories yet
          </h1>
          <p className="text-sm text-muted-foreground">
            Add a local Git repository to see how its branches relate. Open as
            many as you like — each one gets its own tab.
          </p>
        </div>
        <Button onClick={onAdd} disabled={picking}>
          <Plus aria-hidden="true" />
          Add repository
        </Button>
      </div>
    </div>
  );
}
