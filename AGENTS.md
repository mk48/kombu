# AGENTS.md

Guidance for AI coding agents working in this repository.

## What kombu is

kombu is a desktop app that **visualizes the branch topology of a Git repository** — not its
commit history.

A DevOps engineer opens a repo and immediately sees:

- how many branches exist, and what kind they are (trunk, release, feature, hotfix, …)
- which branch each branch was **cut from** (its fork parent)
- which branches have been **merged into where**, and which are still open
- the overall shape of the branching model in use (trunk-based, GitFlow, …)

### The core visual metaphor

- Every branch owns **exactly one horizontal track (lane)**. A branch never changes lane, and
  no two branches share a lane. This is the defining constraint of the whole UI — it is what
  makes the picture readable at 50+ branches, and it is what separates kombu from every
  commit-graph viewer (`gitk`, `git log --graph`, GitLens), which multiplex lanes to save space.
- The **X axis** is progression (time or topological order). The **Y axis** is just lane index —
  it carries no meaning beyond ordering, so lanes can be freely reordered.
- Individual commits are **not** the subject. A branch is drawn as a line/bar, annotated at
  most with counts. Only topology-bearing points get a marker: the fork point where it was
  created, and each merge in or out.
- Connectors between lanes are the payload: a **fork edge** (branch A cut from branch B) and a
  **merge edge** (branch A merged into branch B). Direction must be unambiguous.
- The user **scrolls / pans lanes up and down** to bring branches of interest into view, and
  reorders or pins lanes. With many branches only a window of lanes is visible at a time, so
  vertical navigation is a first-class interaction, not an afterthought.

Keep these properties intact when implementing features. If a change would make a branch
occupy two lanes, or would turn the view into a commit graph, it is the wrong change.

## Current state

**The lane view exists and covers fork edges, merge edges, and manual reordering.**

Implemented:

- Adding a local repository through the native folder picker, from the tab strip's plus button
  or the empty state; multiple repositories open at once, one tab each; persistence to a JSON
  file, so tabs, selection, and lane order all come back on the next launch.
- Reading a repository's origin branches (`git.go`'s `readBranches`, from `refs/remotes/origin/*`
  — not local branches, see "Decisions made") and merge-into-where edges via first-parent walks
  (`readMergeEdges`, with `filterInheritedMerges` dropping merges a branch only "has" because it
  descends from the default branch's history — see that function's doc comment for why that
  matters at scale).
- Fork-parent inference (`inferForkEdges`): a heuristic — see the domain notes below — based on
  pairwise `merge-base`, picking whichever candidate's merge-base is furthest downstream on the
  branch's own ancestry chain. The default branch is never assigned a parent. A branch with no
  confident candidate gets no edge, not a guess.
- The lane renderer itself (`frontend/src/components/branch-tree/`): one fixed horizontal lane
  per branch on a shared calendar-time X axis, hand-rolled SVG (see "Decisions made" — this was
  the "Open decisions" rendering question, now settled), manually reorderable via `@dnd-kit`
  drag handles with the order persisted per repository (`Repo.LaneOrder` in `store.go`,
  `WorkspaceService.SetLaneOrder`). Shows branch bars, merge connectors (solid, arrowhead,
  landing at the merge timestamp), and fork connectors (dashed, hollow arrowhead, since they're
  inferred rather than observed) between lanes.

Not implemented — squash/rebase-merge detection, backfilling a merge edge's `From` when the
source branch has been deleted from origin, an active/deleted-branch visibility toggle (blocked
on that same backfill — there's currently no way to know a branch used to exist), and lane
virtualization (the scrollable container has no windowing yet; fine at the branch counts tested
so far, worth revisiting if a very large repo turns out to lag).

Template leftovers still to deal with (cosmetic, so do not churn them in unrelated PRs):

- `build/config.yml` still holds the `My Company` / `My Product` / `com.mycompany.myproduct`
  placeholders. Update it and then run `wails3 task common:update:build-assets`, which
  regenerates the platform assets under `build/` from it — expect that to touch several files.
- `frontend/public/wails.png` is still the window/tab icon, as is `build/appicon.png`.
- `frontend/public/Inter-Medium.ttf` is unused; the UI renders in Geist, loaded via
  `@fontsource-variable/geist` in `src/index.css`.

## Stack

| Layer | Choice |
|---|---|
| Shell | Wails v3 (`v3.0.0-beta.3`) — beta, APIs can shift between betas |
| Backend | Go 1.25 |
| Frontend | React 18 + TypeScript 5, Vite 8 |
| Styling | Tailwind CSS v4 (CSS-first, no `tailwind.config.js`) |
| Components | shadcn (`style: base-nova`, `baseColor: neutral`), Base UI, lucide icons |
| Build | Taskfile (`task` / `wails3 task`) |

## Layout

```
main.go                     app + window setup, service registration, embeds frontend/dist
workspaceservice.go         the only Wails service: folder picker + repository shelf + branches
store.go                    Repo/Workspace model (incl. LaneOrder), repo discovery, persistence
git.go                      shells out to git: branches, merge edges, fork-parent inference
lanes.go                    reconcileLaneOrder — saved lane order + live branches -> render order
*_test.go                   tests for the above
Taskfile.yml                top-level tasks; dispatches to build/<goos>/Taskfile.yml
build/                      per-platform build assets + config.yml — generated, see below
frontend/
  index.html                sets class="dark" on <html>; no stylesheet link of its own
  src/main.tsx              React root — imports index.css, which is what loads Tailwind
  src/App.tsx               shell: tab strip + notice bar + panel
  src/index.css             Tailwind v4 entry + design tokens (oklch, light + .dark)
  src/hooks/use-workspace.ts    all frontend state and every service call
  src/components/repo-tabs.tsx      tab strip, plus button, keyboard navigation
  src/components/repo-panel.tsx     selected repository: header + the branch tree
  src/components/branch-tree/       the lane view — see its own files for the geometry model:
    branch-tree.tsx           composes the scroll container, DndContext, ruler, gutter, plot
    lane-row.tsx               one draggable HTML row in the label gutter
    lane-bars.tsx               SVG: one line per branch, spanning its known timestamps
    merge-connectors.tsx        SVG: solid curves + arrowhead between merge lanes
    fork-connectors.tsx         SVG: dashed curves + hollow arrowhead for inferred fork edges
    time-ruler.tsx              sticky calendar-time axis header
    time-scale.ts / geometry.ts   the shared coordinate math the above all read from
  src/components/empty-workspace.tsx / notice-bar.tsx
  src/components/ui/        shadcn components (button.tsx only so far)
  src/lib/utils.ts          cn() helper
  bindings/kombu/           GENERATED TypeScript bindings — never hand-edit
  public/                   static assets (only the icon and an unused font remain)
```

### How the frontend talks to Go

`use-workspace.ts` is the only place that calls the service; components take props. Every
mutating service method returns the **whole** `Workspace`, and the hook replaces its state from
that one value rather than patching locally — so the persisted file is always the single source
of truth and the UI cannot drift from it. Keep that property when adding methods.

Two shapes to remember when consuming bindings:

- A nil Go slice arrives as `null`, so `Workspace.repos` is typed `Repo[] | null`. Normalise it
  (`?? []`) at the boundary.
- `CancellablePromise.cancel()` **rejects** the promise with a `CancelError`. Any `.catch` that
  surfaces errors to the user must ignore `err.name === "CancelError"`, or React 18's
  StrictMode double-mount will flash a spurious error in dev.
- Service calls can resolve out of order (two quick tab clicks; the plus button pressed before
  the initial load lands). Every request in the hook takes a token from a `latest` ref and only
  the newest may write state. New calls must follow that pattern, or a slow earlier reply will
  clobber a newer one.

## Commands

Run from the repo root. `wails3` and `task` must be on PATH.

```powershell
wails3 dev                  # dev mode: Go rebuild + Vite HMR (Vite on port 9245)
wails3 build                # production binary into bin/
wails3 task run             # run the built binary
wails3 task package         # platform installer/bundle
wails3 generate bindings -ts -i -clean=true   # regenerate frontend/bindings after Go changes
wails3 task test            # go test ./...
wails3 task check           # go vet . + frontend tsc --noEmit
go build .                  # quick backend compile check
```

Note `go build ./...` fails with `function main is undeclared in the main package` from
`build/ios` — that is the template's iOS scaffolding, which only compiles under its own build
tags. Use `go build .` for the app. `go test ./...` is unaffected.

There is no frontend test runner. If you add one, wire it into `Taskfile.yml` and note it here.

## Rules

1. **Never edit `frontend/bindings/`.** It is regenerated by `wails3 generate bindings`, which
   runs with `-clean=true` and deletes the directory first. Change the Go service, regenerate,
   then use the new binding.
2. **Every backend capability is a Wails service method.** Add a struct with exported methods,
   register it in `application.Options.Services` in `main.go`, regenerate bindings, then call it
   from TypeScript via the generated module. Do not reach for HTTP endpoints.
3. **`main.go` has `//go:embed all:frontend/dist`.** A Go build fails if `frontend/dist` is
   missing, so build the frontend first (the Taskfile deps already do this) — don't delete
   `dist` and then run a bare `go build`.
4. **Tailwind v4 is CSS-first.** Theme changes go in `frontend/src/index.css` under `@theme
   inline` / `:root` / `.dark`. Do not add a `tailwind.config.js`.
5. **Use the design tokens**, not raw colors: `bg-background`, `text-foreground`, `border-border`,
   `chart-1..5`, etc. Dark mode is class-based (`.dark` on an ancestor) via
   `@custom-variant dark`.
6. **Import with the `@/` alias** (`@/components/ui/button`), configured in both
   `vite.config.ts` and `tsconfig.app.json`.
7. **TS strictness:** `strict: true` and `noUnusedLocals: true` are on, so unused imports break
   the build; `noImplicitAny` is off, but still type new code.
8. `build/**` is largely generated by `wails3 update build-assets` from `build/config.yml`.
   Edit `config.yml`, then regenerate — hand-edits to generated assets get overwritten.
9. **Long-running Go work must not block the UI thread.** Reading a large repo's refs is slow;
   either return from a service method promptly or push progress through
   `app.Event.Emit(...)` and listen with `Events.On(...)` on the frontend (see `App-bak.tsx`).
10. Dev host is Windows with PowerShell. Prefer `filepath.Join` over string concatenation, and
    expect `\` in paths and `.exe` suffixes.
11. **Compare repository paths through `pathKey`**, never with `==`. Windows and macOS default
    to case-insensitive filesystems, so `C:\Repo` and `c:\repo` are one directory and must not
    become two tabs.
12. **Never let a bad preferences file stop the app.** `store.load` moves an unparseable
    workspace file aside and starts empty, and `ServiceStartup` logs rather than returns that
    failure. A repository whose folder has gone missing is flagged (`Repo.Missing`) and greyed
    out, never silently dropped — an unplugged drive must not delete the user's tabs.

## Persisted state

The workspace lives in a single JSON file, deliberately not a database:

- `%AppData%\kombu\workspace.json` on Windows, `os.UserConfigDir()/kombu/` elsewhere.
- Written through a `.tmp` file and `os.Rename`, so an interrupted write cannot truncate it.
- `Version` is stamped on every write; bump `storeVersion` and migrate on load if the shape of
  `Workspace` changes.
- `Repo.ID` is a 12-character SHA-256 prefix of the case-folded path — stable across restarts,
  so it is safe as a React key and as the handle passed back to service calls.
- `Repo.Missing` is recomputed on every read, so whatever value is on disk is ignored.

Repository discovery (`repoRoot`) walks **up** from the chosen folder, matching how the git CLI
behaves: picking `myrepo/src/lib` adds `myrepo`. It accepts a `.git` directory, a `.git` *file*
(worktrees and submodules), and bare repositories (`HEAD` + `objects/` + `refs/`). It does this
itself, without shelling out — `git.go` is the only place that does, see "Decisions made" below.

## Domain notes for whoever implements the Git reading

These are the hard parts of the problem, recorded so they don't get rediscovered:

- **Branch tips** are cheap: enumerate `refs/remotes/origin/*` (see "Decisions made" — kombu
  shows origin's branches, not local ones, so this is settled: no local/remote fold to design).
- **"Merged into where"** is derivable and reliable: a merge commit has ≥2 parents; its first
  parent is the branch that received the merge, the others are what was merged in. Walking
  first-parent chains from each tip gives merges *into* that branch. `git branch --merged <ref>`
  gives the cheap "is it already merged" answer for the open/closed distinction.
- **"Cut from which branch" is a heuristic, not a fact.** Git stores no parent-branch pointer.
  Options, roughly in order of reliability: (a) the merge commit message conventions left by
  hosting platforms, (b) `git merge-base A B` across candidate branches, picking the candidate
  whose merge-base is nearest the branch's first unique commit, (c) `git reflog` / `merge-base
  --fork-point`, which is accurate locally but is empty in a fresh clone and expires. Option (b)
  is what's implemented (`inferForkEdges` in `git.go`): pairwise `merge-base` against every other
  branch, picking whichever candidate's merge-base is furthest downstream on the branch's own
  ancestry — see that function's doc comment for the direction-of-parenthood subtlety this
  requires. The UI marks these edges visually as inferred (dashed, hollow arrowhead, a tooltip
  saying "(inferred)") but there's still no way for a user to *correct* a wrong guess — that's
  outstanding, and matters because silently guessing wrong about a fork parent is the worst
  failure mode this app has.
- **Squash-merged and rebase-merged branches leave no merge commit.** A squash-merged feature
  branch looks unmerged by topology alone. Detect via patch-id equivalence
  (`git cherry`/`--cherry-mark`) or platform conventions, and mark such edges as inferred.
- **Lane assignment** is an ordering problem, not a packing problem — every branch gets a lane,
  so the only question is the order. Keep the ordering pluggable (by fork parent then creation
  time is a sensible default: it puts children adjacent to their parent) and stable across
  refreshes, so the picture doesn't reshuffle when a branch is added.
- **Scale:** monorepos routinely have hundreds of branches. Assume lane virtualization is
  needed and that a full topology walk is too slow to redo on every render — compute the model
  once in Go, hand a compact structure to the frontend, and recompute only on refresh.

## Decisions made

- **Rendering:** hand-rolled SVG (`frontend/src/components/branch-tree/`), not a graph-layout
  library (react-flow/dagre/cytoscape fight the fixed-one-lane-per-branch constraint, since they
  assume free layout) and not canvas (SVG's crisp hit-testing and `<title>` tooltips won out over
  canvas's raw speed at the branch counts tested). One shared `<svg>` holds every lane's bar and
  every connector in a single `laneY(index)` coordinate space, inside the same native-scrolling
  container as the HTML label gutter — a connector between two far-apart lanes, one of them
  off-screen, is correct by construction with no manual scroll-sync code.
- **Lane ordering:** manual only for now (drag a row's grip handle, via `@dnd-kit`), persisted
  per repository as `Repo.LaneOrder` (`store.go`) through `WorkspaceService.SetLaneOrder`. A
  branch with no saved position sorts after the ones that have one, default branch first, then by
  `CommitterDate` descending (`reconcileLaneOrder` in `lanes.go`) — a stand-in for fork-parent
  grouping as the *default* order, since automatically grouping children next to their inferred
  parent hasn't been built.
- **Repo selection:** native folder picker (`app.Dialog.OpenFile().CanChooseDirectories(true)`),
  reached from the tab strip's plus button. Repositories persist as tabs, so the picker is only
  ever used to add a new one.
- **Persistence:** one JSON file, as described above. No SQLite.
- **Branch source:** origin's branches (`refs/remotes/origin/*`), not local ones. The server
  side is the shared, canonical view of the branching model that a DevOps engineer wants to see —
  a local-only unpushed branch isn't part of that picture. `readBranches` reads whatever
  `refs/remotes/origin/*` already holds; kombu never runs `git fetch` itself, so the view is only
  as fresh as the user's last manual fetch (a "Refresh" action to make that fetch explicit is on
  the punch list below, still undone). No "origin" remote, or nothing fetched yet, yields an
  empty branch list rather than an error.
- **Git access:** shell out to the `git` binary (`git.go`), using machine-readable
  `--format`/NUL-separated output rather than parsing porcelain text, over `go-git`. The domain
  notes below assume CLI-only primitives (`merge-base --fork-point`, `git cherry --cherry-mark`)
  that `go-git` doesn't provide, so shelling out avoids redoing this when fork-parent inference
  and squash-merge detection are built. Requires `git` on PATH, which a Git-visualization app can
  reasonably expect.
- **Theme:** light. Nothing sets a class on `<html>`, so the `:root` token set in `index.css`
  applies and the native window background matches it (`NewRGB(255, 255, 255)`). The `.dark`
  block and the `dark:` variants in `button.tsx` are dormant but intact — a toggle later is only
  a matter of putting `class="dark"` back on the root element and repainting the window colour.

  When styling, beware that **light `--card` and `--background` are both pure white**. Anything
  that needs to read as raised or recessed against the page cannot rely on those two differing;
  the tab strip uses `bg-muted` for exactly this reason. Hover states on tinted surfaces use
  foreground alpha (`hover:bg-foreground/5`) rather than `hover:bg-muted`, which would be
  invisible on a muted background.

## Open decisions

Not yet settled. Pick deliberately and record the choice here.

- Whether tags and worktrees appear in the view at all (remote branches: settled, see "Decisions
  made" — they're the only branches shown).
- Lane virtualization: not yet needed at the branch counts tested, but the geometry model (one
  shared SVG, absolute `laneY(index)` positioning) was deliberately built so that windowing which
  elements mount later doesn't require changing the coordinate system.

## Where to pick up

The shelf and the lane view both work; refinement and the remaining heuristics are what's left.
In rough order:

1. ~~Read a repository's branch list into a Go model and surface it in `RepoPanel` as a plain
   list.~~ Done: `git.go`'s `readBranches`/`readMergeEdges`, `WorkspaceService.GetBranches`.
2. ~~Fork-parent inference, lane ordering, and the renderer.~~ Done: `inferForkEdges`, lane
   ordering (`lanes.go`, `Repo.LaneOrder`), `frontend/src/components/branch-tree/`.
3. Squash/rebase-merge detection, and backfilling a merge edge's `From` when the source branch
   has been deleted — the latter also blocks a real active/deleted-branch visibility toggle, since
   there's currently no way to know a branch used to exist at all.
4. Vertical navigation refinements over lanes (virtualization, jump-to-branch) if a large repo
   turns out to need them; lane hit-testing / click-to-focus interactions once commits are shown.

Cheap things left undone in the current UI, none of them blocking: a refresh action, tab
reordering by drag, renaming a repository's label, `Ctrl`/`Cmd`+`T` and `+W` shortcuts, and a
"remove missing repositories" affordance.

## Committing

`main` is the only branch and the working tree is clean at time of writing. Don't commit or
push unless asked. Branch before committing to `main`.
