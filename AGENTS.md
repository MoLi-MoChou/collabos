# AGENTS.md

Guidance for coding agents working in this repository. Full rules: `docs/collabos-engineering-conventions.md`.

## Layout
- `server/` Go module. Business logic in `internal/service`, HTTP/WS in `internal/handler`, pure rules in
  `internal/{taskstate,delegation,approval}` (unit-tested, no I/O). DB access only through sqlc (`pkg/db/queries/*.sql` →
  `sqlc generate`). Wire protocol in `pkg/protocol`; after changes run `pnpm gen:protocol` (tygo → `packages/core/src/realtime/protocol.gen.ts`).
- Daemon code under `server/internal/daemon/*` and `cmd/collab`.
- Frontend: `packages/ui` (tokens + components), `packages/core` (data), `packages/views` (screens), `apps/web` (shell).

## Rules
- Never add Git credentials or merge logic to the server; merges run on the approver's daemon.
- Migrations are append-only; new indexes use `CREATE INDEX CONCURRENTLY` in their own migration.
- The audit table is append-only (trigger enforced); every human decision writes an audit row.
- Frontend: server data lives in TanStack Query; realtime events patch/invalidate the cache. zustand is client state only.
  Validate every response with zod (`parseWithFallback`). No `as` casts. Views never import a router, zustand or Tauri.
- Styling: tokens only (`bg-surface`, `text-fg-muted`, `text-body`, `rounded-card` …). No hex, palette classes or arbitrary values.
- All UI text goes through i18n (`zh-Hans.json` and `en.json` must keep identical keys).
- Use Conventional Commits.

## Verify before finishing
```
cd server && go vet ./... && go test ./...
pnpm typecheck && pnpm lint && pnpm test && pnpm build
pnpm e2e   # when touching flows
```
