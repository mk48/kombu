import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { WorkspaceService } from "../../bindings/kombu";
import type { Branch, BranchInfo, Repo, Workspace } from "../../bindings/kombu";

export type Notice = {
  tone: "error" | "info";
  message: string;
};

/**
 * useWorkspace holds the repositories shown as tabs and mediates every call to the
 * Go side. The backend returns the whole workspace from each mutation, so state is
 * always replaced from one authoritative value rather than patched locally.
 */
export function useWorkspace() {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [activeId, setActiveId] = useState("");
  const [loading, setLoading] = useState(true);
  const [picking, setPicking] = useState(false);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [branchInfo, setBranchInfo] = useState<BranchInfo | null>(null);
  // Set optimistically the moment a lane drag is dropped, and cleared once the
  // backend confirms the reorder (or reverted if the save fails), so dragging
  // feels instant without abandoning "the backend's state is the truth".
  const [pendingLaneOrder, setPendingLaneOrder] = useState<string[] | null>(
    null,
  );

  // The folder picker is a native modal window. Guard with a ref as well as state
  // so a double-click cannot open two of them before React re-renders.
  const pickingRef = useRef(false);

  // Responses can arrive out of order — clicking two tabs quickly, or hitting the
  // plus button before the initial load has landed. Each request takes a token and
  // only the newest one is allowed to write state, so a slow earlier reply cannot
  // clobber a newer one.
  const latest = useRef(0);

  const apply = useCallback((token: number, workspace: Workspace) => {
    if (token !== latest.current) return;
    // A nil Go slice arrives as null.
    setRepos(workspace.repos ?? []);
    setActiveId(workspace.activeId);
  }, []);

  const fail = useCallback((err: unknown) => {
    if (isCancellation(err)) return;
    setNotice({ tone: "error", message: describe(err) });
  }, []);

  useEffect(() => {
    const token = ++latest.current;
    const pending = WorkspaceService.GetWorkspace();
    pending
      .then((workspace) => apply(token, workspace))
      .catch(fail)
      .finally(() => {
        // StrictMode mounts, unmounts and remounts in dev, cancelling the first
        // request. Clearing `loading` from that dead request would flash the
        // empty state before the live one lands.
        if (token === latest.current) setLoading(false);
      });
    return () => {
      pending.cancel();
    };
  }, [apply, fail]);

  // Branches and merge edges are live Git state, re-read whenever the selected
  // tab changes, rather than part of the persisted Workspace.
  const latestBranches = useRef(0);
  useEffect(() => {
    setBranchInfo(null);
    setPendingLaneOrder(null);
    if (!activeId) return;
    const token = ++latestBranches.current;
    const pending = WorkspaceService.GetBranches(activeId);
    pending
      .then((info) => {
        if (token === latestBranches.current) setBranchInfo(info);
      })
      .catch(fail);
    return () => {
      pending.cancel();
    };
  }, [activeId, fail]);

  // Informational notices are transient; errors stay until the user dismisses them.
  useEffect(() => {
    if (notice?.tone !== "info") return;
    const timer = setTimeout(() => setNotice(null), 4000);
    return () => clearTimeout(timer);
  }, [notice]);

  const addRepository = useCallback(async () => {
    if (pickingRef.current) return;
    pickingRef.current = true;
    setPicking(true);
    setNotice(null);
    try {
      const result = await WorkspaceService.AddRepository();
      // The picker is modal and can be open for a long time, so claim the token
      // on the way out rather than on the way in.
      apply(++latest.current, result.workspace);
      if (result.duplicate && result.repo) {
        setNotice({
          tone: "info",
          message: `${result.repo.name} is already open — switched to that tab.`,
        });
      }
    } catch (err) {
      fail(err);
    } finally {
      pickingRef.current = false;
      setPicking(false);
    }
  }, [apply, fail]);

  const removeRepository = useCallback(
    async (id: string) => {
      const token = ++latest.current;
      try {
        apply(token, await WorkspaceService.RemoveRepository(id));
      } catch (err) {
        fail(err);
      }
    },
    [apply, fail],
  );

  const selectRepository = useCallback(
    async (id: string) => {
      const token = ++latest.current;
      // Switching tabs must feel instant, so move the selection now and let the
      // backend confirm it.
      setActiveId(id);
      try {
        apply(token, await WorkspaceService.SetActiveRepository(id));
      } catch (err) {
        fail(err);
      }
    },
    [apply, fail],
  );

  // The backend keeps activeId pointing at a real repo; falling back to the first
  // one means a stale id can never leave a populated workspace looking empty.
  const activeRepo =
    repos.find((repo) => repo.id === activeId) ?? repos[0] ?? null;

  // The lane view's render order: an in-flight drag's order takes priority so
  // dropping feels instant, falling back to the server-reconciled order that
  // GetBranches always returns fully populated.
  const lanes = useMemo(() => {
    const branches = branchInfo?.branches ?? [];
    const order = pendingLaneOrder ?? branchInfo?.laneOrder ?? [];
    const byName = new Map(branches.map((branch) => [branch.name, branch]));
    const ordered: Branch[] = [];
    for (const name of order) {
      const branch = byName.get(name);
      if (branch) {
        ordered.push(branch);
        byName.delete(name);
      }
    }
    // Defensive only: laneOrder always names every branch, so nothing should
    // remain in byName here.
    ordered.push(...byName.values());
    return ordered;
  }, [branchInfo, pendingLaneOrder]);

  const reorderLanes = useCallback(
    async (order: string[]) => {
      if (!activeRepo) return;
      setPendingLaneOrder(order);
      const token = ++latest.current;
      const branchesToken = ++latestBranches.current;
      try {
        apply(token, await WorkspaceService.SetLaneOrder(activeRepo.id, order));
        const info = await WorkspaceService.GetBranches(activeRepo.id);
        if (branchesToken === latestBranches.current) {
          setBranchInfo(info);
          setPendingLaneOrder(null);
        }
      } catch (err) {
        // Revert to the last-confirmed order and surface the failure.
        setPendingLaneOrder(null);
        fail(err);
      }
    },
    [activeRepo, apply, fail],
  );

  return {
    repos,
    activeRepo,
    activeId,
    loading,
    picking,
    notice,
    branchInfo,
    lanes,
    merges: branchInfo?.merges ?? [],
    forks: branchInfo?.forks ?? [],
    dismissNotice: useCallback(() => setNotice(null), []),
    addRepository,
    removeRepository,
    selectRepository,
    reorderLanes,
  };
}

/** Cancelling a CancellablePromise rejects it; that is not a failure to report. */
function isCancellation(err: unknown): boolean {
  return err instanceof Error && err.name === "CancelError";
}

function describe(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  if (typeof err === "string" && err) return err;
  return "Something went wrong.";
}
