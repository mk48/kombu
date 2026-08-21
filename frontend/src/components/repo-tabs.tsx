import { useEffect, useRef } from "react";
import { LoaderCircle, Plus, TriangleAlert, X } from "lucide-react";
import type { Repo } from "../../bindings/kombu";
import { cn } from "@/lib/utils";

type RepoTabsProps = {
  repos: Repo[];
  activeId: string;
  picking: boolean;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onAdd: () => void;
};

/**
 * The repository tab strip. One tab per open repository, plus a button to add
 * another. The strip scrolls horizontally when there are more tabs than fit; the
 * add button sits outside that scroll area so it is always reachable.
 */
export function RepoTabs({
  repos,
  activeId,
  picking,
  onSelect,
  onClose,
  onAdd,
}: RepoTabsProps) {
  const tabRefs = useRef(new Map<string, HTMLDivElement | null>());

  // Keep the selected tab visible when the strip has overflowed.
  useEffect(() => {
    tabRefs.current.get(activeId)?.scrollIntoView({
      block: "nearest",
      inline: "nearest",
    });
  }, [activeId]);

  const move = (from: number, delta: number) => {
    if (repos.length === 0) return;
    const next = (from + delta + repos.length) % repos.length;
    const id = repos[next].id;
    onSelect(id);
    tabRefs.current.get(id)?.focus();
  };

  const onKeyDown = (event: React.KeyboardEvent, repo: Repo, index: number) => {
    switch (event.key) {
      case "ArrowLeft":
        event.preventDefault();
        move(index, -1);
        break;
      case "ArrowRight":
        event.preventDefault();
        move(index, 1);
        break;
      case "Home":
        event.preventDefault();
        move(-1, 1);
        break;
      case "End":
        event.preventDefault();
        move(0, -1);
        break;
      case "Delete":
      case "Backspace":
        event.preventDefault();
        onClose(repo.id);
        break;
    }
  };

  return (
    // `bg-muted` rather than `bg-card`: in the light token set --card and
    // --background are both pure white, which would leave the selected tab
    // indistinguishable from the strip behind it.
    <div className="flex items-end gap-1 border-b border-border bg-muted px-2 pt-2">
      <div
        role="tablist"
        aria-label="Open repositories"
        aria-orientation="horizontal"
        // -mb-px lets the selected tab cover the strip's bottom border, so it
        // reads as continuous with the panel below it.
        className="-mb-px flex min-w-0 flex-1 items-end gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {repos.map((repo, index) => {
          const selected = repo.id === activeId;
          return (
            <div
              key={repo.id}
              // A div rather than a button: the close control is a button of its
              // own, and buttons cannot be nested.
              role="tab"
              aria-selected={selected}
              aria-controls="repo-panel"
              id={`repo-tab-${repo.id}`}
              tabIndex={selected ? 0 : -1}
              ref={(node) => {
                tabRefs.current.set(repo.id, node);
              }}
              onClick={() => onSelect(repo.id)}
              onKeyDown={(event) => onKeyDown(event, repo, index)}
              onAuxClick={(event) => {
                // Middle-click closes, as in a browser.
                if (event.button === 1) onClose(repo.id);
              }}
              title={repo.missing ? `${repo.path} (not found)` : repo.path}
              className={cn(
                "group flex h-9 max-w-56 shrink-0 cursor-default items-center gap-2 rounded-t-lg border border-b-0 px-3 text-sm outline-none transition-colors",
                "focus-visible:ring-2 focus-visible:ring-ring/50",
                selected
                  ? "border-border bg-background text-foreground"
                  : "border-transparent text-muted-foreground hover:bg-foreground/5 hover:text-foreground",
              )}
            >
              {repo.missing && (
                <TriangleAlert
                  className="size-3.5 shrink-0 text-destructive"
                  aria-hidden="true"
                />
              )}
              <span className="truncate">{repo.name}</span>
              <button
                type="button"
                aria-label={`Close ${repo.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  onClose(repo.id);
                }}
                className={cn(
                  "-mr-1 grid size-5 shrink-0 place-items-center rounded outline-none transition-opacity",
                  "hover:bg-foreground/10 focus-visible:ring-2 focus-visible:ring-ring/50",
                  selected
                    ? "opacity-70 hover:opacity-100"
                    : "opacity-0 group-hover:opacity-70 group-hover:hover:opacity-100 focus-visible:opacity-100",
                )}
              >
                <X className="size-3.5" aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>

      <button
        type="button"
        onClick={onAdd}
        disabled={picking}
        aria-label="Add repository"
        title="Add a repository"
        className={cn(
          // 4px lifts the button to sit centred against the 36px-tall tabs.
          "mb-1 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground outline-none transition-colors",
          "hover:bg-foreground/5 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50",
          "disabled:pointer-events-none disabled:opacity-50",
        )}
      >
        {picking ? (
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
        ) : (
          <Plus className="size-4" aria-hidden="true" />
        )}
      </button>
    </div>
  );
}
