# kombu

**See the shape of your branching model.**

kombu is a cross-platform desktop app that visualizes the **branch topology** of a Git
repository — deliberately *not* its commit history. Where `git log --graph` and similar tools
show you every commit and weave branches through shared lanes, kombu shows you only the
branches and how they relate, so the branching strategy itself becomes visible at a glance.

Built for DevOps and platform engineers who need to answer, on an unfamiliar repo:

- How many branches are there, and what kinds?
- Which branch was each one **cut from**?
- Which branches have been **merged into where** — and which are still open?
- Are we actually following the branching model we think we are?

## The view

- **One branch, one horizontal track.** A branch stays on its own lane for its whole life and
  never shares it. That single rule is what keeps the picture readable when a repo has fifty
  branches.
- **Horizontal = progression, vertical = just ordering.** Lanes carry no meaning by position,
  so you can reorder and pin them freely.
- **Commits aren't the subject.** Each branch is a line, marked only where topology happens:
  the fork point where it was created, and every merge in or out.
- **The connectors are the point.** Fork edges show what came from what; merge edges show what
  landed where, with unambiguous direction.
- **Move up and down through branches** to bring the ones you care about into view.

## Status

🚧 **Early development**, but the core lane view works.

**Working today**

- Add a local Git repository through a native folder picker. Pick any folder inside a
  repository and its root is added, the same way the `git` CLI resolves a repository.
- Keep several repositories open at once, one tab each — add as many as you like. Tabs and the
  selected repository persist between launches, in a plain JSON file under your user config
  directory. A repository whose folder has gone missing is flagged rather than dropped, so an
  unplugged drive doesn't erase your tabs.
- The lane view: one horizontal lane per origin branch, reorderable by dragging its label (the
  order is saved per repository), with merge connectors between lanes and a best-guess "cut from"
  connector to the branch it was likely forked from.

**Not yet**

Squash/rebase-merge detection, recovering a merge's source branch once it's been deleted from
origin, and a way to correct a wrong fork-parent guess by hand.

## Stack

- **Wails v3** (`v3.0.0-beta.3`) — Go backend, native webview frontend
- **Go 1.25**
- **React 18 + TypeScript 5**, Vite 8
- **Tailwind CSS v4** (CSS-first config) with shadcn components and lucide icons
- **Taskfile** for builds, with per-platform targets including Windows, macOS, Linux, iOS,
  Android, and a server/Docker mode

## Getting started

**Prerequisites:** [Go 1.25+](https://go.dev/dl/), [Node.js](https://nodejs.org/),
[Task](https://taskfile.dev/), and the Wails 3 CLI:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

Then, from the repository root:

```sh
wails3 dev      # run with hot reload (Go rebuilds, Vite HMR on port 9245)
wails3 build    # production binary into bin/
wails3 task run # run the built binary
```

Other useful tasks:

```sh
wails3 task test                                # go test ./...
wails3 task check                               # go vet + frontend typecheck
wails3 task package                             # platform installer / bundle
wails3 generate bindings -ts -i -clean=true     # regenerate TS bindings after Go changes
wails3 task --list                              # everything available
```

## Project layout

```
main.go              Wails app + window setup; embeds frontend/dist
workspaceservice.go  the folder picker and the set of open repositories
store.go             repository discovery and JSON persistence
Taskfile.yml         entry point for builds; dispatches per platform
build/               platform build assets, generated from build/config.yml
frontend/
  src/App.tsx        shell: tab strip and the selected repository's panel
  src/hooks/         use-workspace — all state, and every call into Go
  src/components/    repo-tabs, repo-panel, empty-workspace, notice-bar, ui/
  bindings/          auto-generated TypeScript bindings — do not edit by hand
AGENTS.md            architecture notes, conventions, and domain gotchas
```

Backend capabilities are exposed as Wails **services**: a Go struct registered in `main.go`,
whose exported methods become typed TypeScript functions under `frontend/bindings/`. Those
bindings are generated — change the Go side and regenerate rather than editing them.

## Contributing

Read [AGENTS.md](AGENTS.md) first. It covers the conventions that aren't obvious from the code
(generated directories, Tailwind v4 specifics, keeping the UI responsive during repo scans) and
records the genuinely tricky parts of the domain — notably that "which branch was this cut
from?" is a heuristic Git cannot answer directly, and that squash- and rebase-merged branches
leave no merge commit to find.

## Reference

- [Wails v3 documentation](https://v3.wails.io/)
- [Wails Discord](https://discord.gg/JDdSxwjhGf) · [GitHub discussions](https://github.com/wailsapp/wails/discussions)
