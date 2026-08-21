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

**Repository shelf works; the visualisation does not exist yet.**

Implemented:

- Adding a local repository through the native folder picker, from the tab strip's plus button
  or the empty state.
- Multiple repositories open at once, one tab each, with close buttons and keyboard navigation.
- Persistence to a JSON file, so the same tabs and selection come back on the next launch.

Not implemented — **no Git reading of any kind yet.** A repository's tab shows its folder name
and path and nothing more. Everything this document says about lanes, forks, and merge edges is
*intent*: the model, the Git access layer, and the renderer are all still to be written. Do not
assume a file exists because this document mentions it.

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
workspaceservice.go         the only Wails service: folder picker + repository shelf
store.go                    Repo/Workspace model, repo discovery, JSON persistence
store_test.go               tests for the above
Taskfile.yml                top-level tasks; dispatches to build/<goos>/Taskfile.yml
build/                      per-platform build assets + config.yml — generated, see below
frontend/
  index.html                sets class="dark" on <html>; no stylesheet link of its own
  src/main.tsx              React root — imports index.css, which is what loads Tailwind
  src/App.tsx               shell: tab strip + notice bar + panel
  src/index.css             Tailwind v4 entry + design tokens (oklch, light + .dark)
  src/hooks/use-workspace.ts    all frontend state and every service call
  src/components/repo-tabs.tsx      tab strip, plus button, keyboard navigation
  src/components/repo-panel.tsx     selected repository (placeholder for the lane view)
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
(worktrees and submodules), and bare repositories (`HEAD` + `objects/` + `refs/`). It does not
shell out to `git` — nothing in the codebase does yet, which is one of the open decisions below.

## Domain notes for whoever implements the Git reading

These are the hard parts of the problem, recorded so they don't get rediscovered:

- **Branch tips** are cheap: enumerate `refs/heads/*` (and `refs/remotes/*` if remote branches
  are shown). Decide explicitly whether remote-tracking branches get their own lanes or are
  folded into their local counterpart — showing both doubles the lane count.
- **"Merged into where"** is derivable and reliable: a merge commit has ≥2 parents; its first
  parent is the branch that received the merge, the others are what was merged in. Walking
  first-parent chains from each tip gives merges *into* that branch. `git branch --merged <ref>`
  gives the cheap "is it already merged" answer for the open/closed distinction.
- **"Cut from which branch" is a heuristic, not a fact.** Git stores no parent-branch pointer.
  Options, roughly in order of reliability: (a) the merge commit message conventions left by
  hosting platforms, (b) `git merge-base A B` across candidate branches, picking the candidate
  whose merge-base is nearest the branch's first unique commit, (c) `git reflog` / `merge-base
  --fork-point`, which is accurate locally but is empty in a fresh clone and expires.
  Whichever is chosen, the UI must be able to say "inferred" and let the user correct it —
  silently guessing wrong about a fork parent is the worst failure mode this app has.
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

- **Repo selection:** native folder picker (`app.Dialog.OpenFile().CanChooseDirectories(true)`),
  reached from the tab strip's plus button. Repositories persist as tabs, so the picker is only
  ever used to add a new one.
- **Persistence:** one JSON file, as described above. No SQLite.
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

- **Git access:** shell out to the `git` binary (simple, exact semantics, requires git installed)
  vs. `go-git` (self-contained, pure Go, slower and less complete on odd repos). Nothing reads
  Git yet, so this is wide open — and it is the next thing that has to be decided.
- **Rendering:** SVG (crisp, easy hit-testing, DOM cost at scale) vs. canvas (fast, manual
  hit-testing). Lane virtualization may make SVG viable.
- Whether remote branches, tags, and worktrees appear in the view at all.

## Where to pick up

The shelf is done; the product is not. In rough order:

1. Decide the Git access approach, then read a repository's branch list into a Go model and
   surface it in `RepoPanel` as a plain list. That proves the pipe end to end.
2. Derive the topology: fork parent (a heuristic — see the domain notes) and merge edges.
3. Lane ordering, then the renderer, then vertical navigation over lanes.

Cheap things left undone in the current UI, none of them blocking: a refresh action, tab
reordering by drag, renaming a repository's label, `Ctrl`/`Cmd`+`T` and `+W` shortcuts, and a
"remove missing repositories" affordance.

## Committing

`main` is the only branch and the working tree is clean at time of writing. Don't commit or
push unless asked. Branch before committing to `main`.
