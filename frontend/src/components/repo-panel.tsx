import { FolderGit2, GitBranch, TriangleAlert } from "lucide-react";
import type { BranchInfo, Repo } from "../../bindings/kombu";

/**
 * The content shown for the selected repository. Branches and merge edges are
 * shown as plain lists for now — the lane view that turns this into a tree will
 * replace them later.
 */
export function RepoPanel({
  repo,
  branchInfo,
}: {
  repo: Repo;
  branchInfo: BranchInfo | null;
}) {
  // A nil Go slice arrives as null.
  const branches = branchInfo?.branches ?? [];
  const merges = branchInfo?.merges ?? [];

  return (
    <div
      role="tabpanel"
      id="repo-panel"
      aria-labelledby={`repo-tab-${repo.id}`}
      tabIndex={0}
      className="flex min-h-0 flex-1 flex-col overflow-auto outline-none"
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
      ) : branches.length === 0 ? (
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
        <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-auto px-6 py-5">
          <section>
            <h2 className="text-sm font-medium text-foreground">Branches</h2>
            <ul className="mt-2 divide-y divide-border">
              {branches.map((branch) => (
                <li
                  key={branch.name}
                  className="flex items-center gap-2 py-2 text-sm"
                >
                  <GitBranch
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span
                    className={
                      branch.mergedToDefault
                        ? "text-muted-foreground"
                        : "text-foreground"
                    }
                  >
                    {branch.name}
                  </span>
                  {branch.isDefault && (
                    <span className="rounded-full bg-foreground/10 px-2 py-0.5 text-xs text-foreground">
                      default
                    </span>
                  )}
                  {branch.isCurrent && (
                    <span className="rounded-full bg-foreground/10 px-2 py-0.5 text-xs text-foreground">
                      current
                    </span>
                  )}
                  {branch.mergedToDefault && (
                    <span className="text-xs text-muted-foreground">
                      merged
                    </span>
                  )}
                  <span className="ml-auto font-mono text-xs text-muted-foreground">
                    {branch.head.slice(0, 7)}
                  </span>
                </li>
              ))}
            </ul>
          </section>

          <section>
            <h2 className="text-sm font-medium text-foreground">Merges</h2>
            {merges.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">
                No merge commits found yet.
              </p>
            ) : (
              <ul className="mt-2 divide-y divide-border">
                {merges.map((edge) => (
                  <li
                    key={edge.commit}
                    className="flex items-center gap-2 py-2 text-sm"
                  >
                    <span className="text-foreground">
                      {edge.from || "unknown"}
                    </span>
                    <span className="text-muted-foreground" aria-hidden="true">
                      →
                    </span>
                    <span className="text-foreground">{edge.into}</span>
                    <span className="ml-auto font-mono text-xs text-muted-foreground">
                      {edge.commit.slice(0, 7)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      )}
    </div>
  );
}
