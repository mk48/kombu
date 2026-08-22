import { EmptyWorkspace } from "@/components/empty-workspace";
import { NoticeBar } from "@/components/notice-bar";
import { RepoPanel } from "@/components/repo-panel";
import { RepoTabs } from "@/components/repo-tabs";
import { useWorkspace } from "@/hooks/use-workspace";

function App() {
  const {
    repos,
    activeRepo,
    activeId,
    loading,
    picking,
    notice,
    branchInfo,
    lanes,
    merges,
    dismissNotice,
    addRepository,
    removeRepository,
    selectRepository,
    reorderLanes,
  } = useWorkspace();

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-background text-foreground">
      {/* The strip is always present, even with no tabs in it, so the plus button
          sits in the same place from the first repository onwards. */}
      <RepoTabs
        repos={repos}
        activeId={activeId}
        picking={picking}
        onSelect={selectRepository}
        onClose={removeRepository}
        onAdd={addRepository}
      />

      {notice && <NoticeBar notice={notice} onDismiss={dismissNotice} />}

      {/* Nothing is drawn until the saved workspace has loaded, so that a stored
          set of tabs does not flash the empty state on the way in. */}
      {loading ? (
        <div className="flex-1" />
      ) : activeRepo ? (
        <RepoPanel
          key={activeRepo.id}
          repo={activeRepo}
          branchInfo={branchInfo}
          lanes={lanes}
          merges={merges}
          onReorderLanes={reorderLanes}
        />
      ) : (
        <EmptyWorkspace onAdd={addRepository} picking={picking} />
      )}
    </div>
  );
}

export default App;
