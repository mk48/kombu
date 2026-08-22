import { FolderGit2, GitBranch, TriangleAlert } from "lucide-react";
import type { Branch, BranchInfo, MergeEdge, Repo } from "../../bindings/kombu";
import { BranchTree } from "./branch-tree/branch-tree";

/**
 * The content shown for the selected repository: its header, then the
 * horizontal branch lane tree (or a loading/empty state before it has data).
 */
export function RepoPanel({
  repo,
  branchInfo,
  lanes,
  merges,
  onReorderLanes,
}: {
  repo: Repo;
  branchInfo: BranchInfo | null;
  lanes: Branch[];
  merges: MergeEdge[];
  onReorderLanes: (order: string[]) => void;
}) {
  return (
    <div
      role="tabpanel"
      id="repo-panel"
      aria-labelledby={`repo-tab-${repo.id}`}
      tabIndex={0}
      className="flex min-h-0 flex-1 flex-col outline-none"
    >
      <header className="flex items-start gap-3 border-b border-border px-6 py-5">
        <FolderGit2
          className="mt-0.5 size-6 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="min-w-0">
          <h1 className="truncate text-xl font-semibold tracking-tight">
            {repo.name}
          </h1>
          <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
            {repo.path}
          </p>
        </div>
      </header>

      {repo.missing && (
        <div className="mx-6 mt-5 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <p>
            This folder is no longer on disk. It may live on a drive or share
            that is not currently connected.
          </p>
        </div>
      )}

      {!branchInfo ? (
        <p className="px-6 py-5 text-sm text-muted-foreground">
          Reading branches…
        </p>
      ) : lanes.length === 0 ? (
        <div className="flex flex-1 items-center justify-center px-6 py-16">
          <div className="flex max-w-md flex-col items-center gap-3 text-center">
            <GitBranch
              className="size-8 text-muted-foreground/40"
              aria-hidden="true"
            />
            <p className="max-w-xs text-sm text-muted-foreground">
              No branches found on origin. Make sure this repository has an
              "origin" remote and has been fetched.
            </p>
          </div>
        </div>
      ) : (
        <BranchTree lanes={lanes} merges={merges} onReorder={onReorderLanes} />
      )}
    </div>
  );
}
