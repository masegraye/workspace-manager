Below is a **single, consolidated architecture plan** that fuses the complementary ideas from both refactor proposals and emphasises the pieces you explicitly liked (e.g., `git.Client`).
Everything is phrased as a concrete‑next‑steps guide you can hand to a teammate or ticket into stories.

---

## 0. Guiding Principles

| Principle                                 | Rationale                                                     |
| ----------------------------------------- | ------------------------------------------------------------- |
| **One package ⇢ one responsibility**      | Low cognitive load; smaller files; easier code‑owners.        |
| **Pure domain, impure adapters**          | Business rules in memory; all side‑effects behind interfaces. |
| **Dependency‑injection via small struct** | Swappable fakes → deterministic unit tests.                   |
| **Functional options for public APIs**    | Extensible without breaking existing call‑sites.              |
| **No globals**                            | Testability and predictable behaviour.                        |

---

## 1. Package & Directory Layout

```
pkg/wsm/
├─ domain/        // Pure structs & helpers (NO I/O)
│    └─ types.go  // Workspace, Repository, etc.
│
├─ config/        // Load/save WorkspaceConfig, registry paths
│
├─ fs/            // FileSystem interface + os & afero impls
│
├─ git/           // Thin adapter around 'git' (git.Client)
│
├─ worktree/      // High‑level worktree logic (uses git.Client)
│
├─ metadata/      // Build/write wsm.json & go.work
│
├─ setup/         // Script discovery & execution
│
├─ ux/            // User‑interaction façade (Prompter, Logger)
│
├─ service/       // Orchestration services
│    ├─ workspace.go  // WorkspaceService (create, delete, mutate)
│    └─ sync.go       // SyncService, StatusService, etc.
│
└─ internal/      // Helpers not part of public API
     └─ pathutil/, tempgit/, etc.
cmd/
└─ wsm/           // CLI (cobra, bubbletea, whatever)
```

> **Tip:** keep the top‑level import path stable (`github.com/…/workspace-manager/pkg/wsm/...`) so downstream code does not break during the move.

---

## 2. Cross‑Cutting Interfaces

```go
// fs/file_system.go
package fs
type FileSystem interface {
    MkdirAll(string, os.FileMode) error
    RemoveAll(string) error
    WriteFile(string, []byte, os.FileMode) error
    ReadFile(string) ([]byte, error)
    ReadDir(string) ([]os.DirEntry, error)
    Stat(string) (fs.FileInfo, error)
}

// git/client.go
package git
type Client interface {
    WorktreeAdd(ctx context.Context, repoPath, branch, target string, opts WorktreeAddOpts) error
    WorktreeRemove(ctx context.Context, repoPath, target string, force bool) error
    BranchExists(ctx context.Context, repoPath, branch string) (bool, error)
    RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error)
    AheadBehind(ctx context.Context, repoPath string) (ahead, behind int, err error)
    ListWorktrees(ctx context.Context, repoPath string) ([]WorktreeInfo, error)
}

// ux/prompt.go
package ux
type Prompter interface {
    Select(msg string, options []string) (string, error)
    Confirm(msg string) (bool, error)
}

type Logger interface {
    Info(msg string, kv ...any)
    Warn(msg string, kv ...any)
    Error(msg string, kv ...any)
}

// service/deps.go
package service
type Deps struct {
    FS     fs.FileSystem
    Git    git.Client
    UI     ux.Prompter
    Log    ux.Logger
    Clock  func() time.Time  // overridable in tests
}
```

*Production* implementations live beside the interfaces; tests register fakes/stubs.

---

## 3. Pure Domain Model

Move **all** structs from `types.go` (Workspace, Repository, …) into `pkg/wsm/domain`.
Add innocuous helpers only when they are **pure**—e.g.,

```go
func (w Workspace) NeedsGoWork() bool {
    for _, r := range w.Repositories {
        if slices.Contains(r.Categories, "go") {
            return true
        }
    }
    return false
}
```

No `os`, no `exec`, no printing.

---

## 4. Worktree Module (sample)

```go
type Service struct {
    git git.Client
    fs  fs.FileSystem
    log ux.Logger
}

type CreateOpts struct {
    Branch       string
    BaseBranch   string
    Overwrite    bool
    RemoteExists bool // filled by caller to skip another Git call
}

func (s *Service) Create(
        ctx context.Context,
        repo domain.Repository,
        target string,
        opts CreateOpts,
) error {
    // decide strategy, defer to s.git.WorktreeAdd(...)
    // NOTE: no user prompt, no fmt.Printf, no os.* calls.
}
```

`Service.Delete`, `Service.Rollback` similarly defer to `git.Client`.

---

## 5. Metadata & Go.work Generators

* `metadata.Builder` returns a `[]byte` (`[]byte` → file writer lives in orchestrator).
* `gowork.Generator` given a slice of `Repository` returns the string content.

Pure functions → trivial unit tests.

---

## 6. Setup Service

```go
type Service struct {
    fs  fs.FileSystem
    log ux.Logger
    sh  ExecRunner // thin wrapper, can be mocked
}

func (s *Service) RunAll(ctx context.Context, ws domain.Workspace) error
```

*Collect*, *order*, *execute*, *timeout* logic lives here; no prompts.

---

## 7. WorkspaceService Orchestrator (public entry point)

```go
// service/workspace.go
type WorkspaceService struct {
    Deps      *Deps
    worktrees *worktree.Service
    setup     *setup.Service
    meta      *metadata.Builder
    cfg       config.Loader
}

type CreateRequest struct {
    Name         string
    RepoNames    []string
    Options      []CreateOption // functional options
}

func (s *WorkspaceService) Create(
        ctx context.Context,
        req CreateRequest,
) (*domain.Workspace, error) {

    // 1. validate & compute paths (pure steps)
    // 2. deps.FS.MkdirAll(workspacePath, 0755)
    // 3. resolve repositories via discoverer
    // 4. loop repos → worktrees.Create(...)
    //    keep rollback slice
    // 5. if NeedsGoWork → gowork.Generator + FS.WriteFile
    // 6. meta.Builder → FS.WriteFile(".wsm/wsm.json")
    // 7. setup.RunAll(...)
    // 8. cfg.Save(workspace)
    // 9. return workspace
}
```

Rollback is merely `for created []*repo { worktrees.Remove(force) }`.

---

## 8. Functional‑Option Setters

```go
// workspace/options.go
package workspace

type CreateOption func(*CreateRequest)

func WithBranch(b string) CreateOption      { return func(r *CreateRequest) { r.Branch = b } }
func WithBaseBranch(b string) CreateOption  { return func(r *CreateRequest) { r.BaseBranch = b } }
func DryRun(v bool) CreateOption            { return func(r *CreateRequest) { r.DryRun = v } }
```

Call‑site:

```go
ws, err := wsSvc.Create(ctx,
        workspace.CreateRequest{
            Name:      "acme-dev",
            RepoNames: []string{"api", "cli", "web"},
        },
        workspace.WithBranch("feature/foo"),
        workspace.DryRun(true),
)
```

---

## 9. CLI / Command Layer

The **only** place that imports `ux.Prompter` directly.
Typical flow:

1. Gather flags/env.
2. If `--force` not supplied and a branch clash is detected, call `deps.UI.Select(...)`.
3. Build a `CreateRequest` and invoke `WorkspaceService`.

Because prompting lives here, lower layers stay deterministic.

---

## 10. Test Plan

| Layer            | Test Type      | Tooling                                        |
| ---------------- | -------------- | ---------------------------------------------- |
| domain           | unit           | plain `go test`                                |
| git.Client       | integration    | Temporary `exec.Command` against real Git repo |
| worktree.Service | unit           | fake git.Client + afero mem FS                 |
| metadata/gowork  | unit           | compare generated bytes/strings                |
| setup.Service    | unit           | stub ExecRunner + scripted FS                  |
| WorkspaceService | unit & integ   | unit with fakes; integ with real Git + temp FS |
| CLI command      | e2e (optional) | `os/exec` calling compiled binary              |

The fakes live in `pkg/wsm/testkit/…` to avoid import loops.

---

## 11. Migration Checklist (incremental)

| Step | Action                                                                                                                                       |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Create **domain/** and move structs; adjust imports (mechanical).                                                                            |
| 2    | Introduce `fs.FileSystem` interface and wrap `os` calls in current code; main code still compiles.                                           |
| 3    | Extract thin `git.Client` that simply forwards to `exec.CommandContext`; swap current direct calls.                                          |
| 4    | Carve out `worktree.Service` (move `createWorktree`, `removeWorktree`, rollback helpers).                                                    |
| 5    | Move `createGoWorkspace` → `gowork.Generator`; `createWorkspaceMetadata` → `metadata.Builder`.                                               |
| 6    | Lift prompting code to CLI layer (`cmd/wsm`); keep temporary shim calling Prompter from inside functions—to be removed once callers migrate. |
| 7    | Implement `WorkspaceService.Create` using new modules; delete old functions from `workspace.go`.                                             |
| 8    | Migrate remaining operations (AddRepo, RemoveRepo, DeleteWorkspace, Sync) similarly.                                                         |
| 9    | Delete the monolithic file; run `go vet`, `staticcheck`, `golangci-lint`.                                                                    |

At the end **`workspace.go` should contain < 150 LOC**, primarily:

```go
type Manager struct {
    svc *service.WorkspaceService
}
```

---

## 12. Immediate Code Stubs (to get you started)

```go
// git/exec_client.go
package git

type execClient struct{}

func NewExecClient() Client { return &execClient{} }

func (c *execClient) WorktreeAdd(ctx context.Context, repo, branch, tgt string, o WorktreeAddOpts) error {
    args := []string{"worktree", "add"}
    if o.Overwrite { args = append(args, "-B", branch) } else if branch != "" { args = append(args, "-b", branch) }
    args = append(args, tgt)
    if o.RemoteRef != "" { args = append(args, o.RemoteRef) }
    return run(ctx, repo, "git", args...)
}
// ... other methods ...
```

```go
// fs/os_fs.go
package fs

type OS struct{}

func (OS) MkdirAll(p string, m os.FileMode) error  { return os.MkdirAll(p, m) }
func (OS) WriteFile(p string, b []byte, m os.FileMode) error { return os.WriteFile(p, b, m) }
...
```

```go
// ux/std_logger.go
type StdLogger struct{}
func (l StdLogger) Info(msg string, kv ...any)  { log.Printf("INFO: "+msg, kv...) }
func (l StdLogger) Warn(msg string, kv ...any)  { log.Printf("WARN: "+msg, kv...) }
func (l StdLogger) Error(msg string, kv ...any) { log.Printf("ERR : "+msg, kv...) }
```

These stubs compile immediately and can be swapped out later with richer impls.

---

### End‑State Benefits

* **Maintainability** – clear ownership tree; each package < 300 LOC.
* **Testability** – > 90 % logic covered by unit tests without shelling out.
* **Extensibility** – to add “mirror to GitHub”, introduce a new `scm.Client` and call it from `service.WorkspaceService`—no god‑file surgery.
* **Operational clarity** – all logging routes through one injected `Logger`, enabling structured logs or OpenTelemetry spans by config only.

---

**Ready‑to‑start summary**

1. Scaffold `domain`, `fs`, `git`, `ux` packages with interfaces and thin default implementations.
2. Create `service.Deps` and wire it in `cmd/wsm`.
3. Extract `worktree.Service` first—it is the highest‑risk piece and unlocks rollback simplification.
4. Move metadata/gowork into pure functions; update callers.
5. Replace residual direct `os` / `exec` in old code with injected deps.
6. Delete the god‑file.

Feel free to adjust the names, but **keep the boundaries**—they provide the enduring value.
