# Stage 4 Community Interactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add searchable public video discovery, author-controlled publishing, likes, favorites, comments, follows, watch history, statistics, and the corresponding frontend workflows.

**Architecture:** Keep the existing Gin modular monolith and Handler → Service → Repository layering. MySQL transactions and unique keys provide synchronous correctness; `video` owns publishing/read models, `engagement` owns viewer actions and history, and `follow` owns the user graph. The native JavaScript frontend uses focused API clients and renders the new workflows inside the existing application shell.

**Tech Stack:** Go, Gin, GORM, Goose, MySQL 8, native HTML/CSS/JavaScript, Node test runner, MinIO, Kafka, HLS.js.

---

### Task 1: Stage 4 schema and domain models

**Files:**
- Create: `backend/migrations/000004_add_community_schema.sql`
- Create: `backend/migrations/000005_backfill_community_data.sql`
- Modify: `backend/internal/video/model.go`
- Create: `backend/internal/engagement/model.go`
- Create: `backend/internal/follow/model.go`
- Create: `backend/internal/database/community_migrate_integration_test.go`

- [x] **Step 1: Write a failing migration integration test**

Create an isolated database with `testutil.MySQLDSN(t)`, run all migrations, and assert that `videos.visibility`, `videos.published_at`, `video_stats`, `video_likes`, `video_favorites`, `comments`, `user_follows`, and `watch_histories` exist. Insert duplicate like/favorite/follow rows and assert MySQL rejects them; insert a self-follow and assert the check constraint rejects it.

```go
func TestCommunitySchemaConstraints(t *testing.T) {
    db := openTestDB(t)
    migrator, err := NewMigrator(db)
    if err != nil { t.Fatal(err) }
    if _, err := migrator.Up(context.Background()); err != nil { t.Fatal(err) }
    var columns int
    err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = DATABASE() AND table_name = 'videos'
        AND column_name IN ('visibility', 'published_at')`).Scan(&columns)
    if err != nil { t.Fatal(err) }
    if columns != 2 { t.Fatalf("community video columns = %d, want 2", columns) }
}
```

- [x] **Step 2: Run the test and verify RED**

Run: `go test -tags=integration ./internal/database -run TestCommunitySchemaConstraints -count=1`

Expected: FAIL because migration `000004` and the community tables do not exist.

- [x] **Step 3: Add schema and data migrations**

`000004` adds `visibility TINYINT NOT NULL DEFAULT 1` and nullable `published_at` to `videos`, then creates the six community tables using InnoDB, `utf8mb4_unicode_ci`, foreign keys, composite unique/primary keys, count checks, and list indexes. Its Down section drops dependent tables before the two video columns.

`000005` contains only data migration statements:

```sql
UPDATE videos
SET published_at = COALESCE(processed_at, updated_at, created_at)
WHERE status = 2 AND visibility = 1 AND published_at IS NULL;

INSERT INTO video_stats (video_id)
SELECT id FROM videos
ON DUPLICATE KEY UPDATE video_id = VALUES(video_id);
```

Its Down section is an explicitly documented no-op because published timestamps cannot safely be distinguished from values created after deployment.

- [x] **Step 4: Add matching Go models and run GREEN**

Add `VisibilityPublic=1`, `VisibilityPrivate=2`, `VideoStats`, `Like`, `Favorite`, `Comment`, `WatchHistory`, and `Follow` models with explicit `TableName()` methods. Run:

`gofmt -w internal/video/model.go internal/engagement/model.go internal/follow/model.go internal/database/community_migrate_integration_test.go`

`go test -tags=integration ./internal/database -run TestCommunitySchemaConstraints -count=1`

Expected: PASS.

- [x] **Step 5: Commit the schema checkpoint**

```bash
git add backend/migrations backend/internal/video/model.go backend/internal/engagement/model.go backend/internal/follow/model.go backend/internal/database/community_migrate_integration_test.go
git commit -m "feat: add community interaction schema"
```

### Task 2: Searchable public video read model

**Files:**
- Modify: `backend/internal/video/model.go`
- Modify: `backend/internal/video/dto.go`
- Modify: `backend/internal/video/repository.go`
- Modify: `backend/internal/video/service.go`
- Modify: `backend/internal/video/handler.go`
- Modify: `backend/internal/video/service_test.go`
- Modify: `backend/internal/video/handler_test.go`
- Create: `backend/internal/video/repository_integration_test.go`

- [x] **Step 1: Write failing tests for query validation and repository search**

Cover `ListQuery{Page, PageSize, Query, Sort}` with empty search, Chinese title search, author nickname search, literal `%`/`_` search, `latest`, `popular`, invalid sort, a 51-rune query, and exclusion of private/non-ready/deleted videos.

```go
type ListQuery struct {
    Page int
    PageSize int
    Query string
    Sort Sort
}
```

Run: `go test ./internal/video -run 'Test(ListQuery|ListHandler)' -count=1`

Expected: FAIL because `ListQuery`, search, sort, stats, and visibility do not exist.

- [x] **Step 2: Implement the public query and read model**

Change the repository contract to:

```go
ListPublic(ctx context.Context, query ListQuery) ([]Video, int64, error)
FindPublicByID(ctx context.Context, id uint64) (*Video, error)
ViewerState(ctx context.Context, viewerID, videoID, authorID uint64) (ViewerState, error)
```

Join `users` and left join `video_stats`, filter `status=ready AND visibility=public`, and bind a pattern escaped with `!`:

```go
pattern := "%" + escapeLike(query.Query, '!') + "%"
db.Where(`v.title LIKE ? ESCAPE '!' OR v.description LIKE ? ESCAPE '!'
          OR u.username LIKE ? ESCAPE '!' OR u.nickname LIKE ? ESCAPE '!'`,
    pattern, pattern, pattern, pattern)
```

Sort `latest` by `COALESCE(published_at, created_at), id`; sort `popular` by `view_count`, `like_count`, then publication time. Extend responses with `visibility`, `published_at`, `stats`, and optional `viewer_state`.

Change `Repository.Create` to create the video and its zeroed `video_stats` row in one transaction, so videos created after the backfill have the same invariant as existing rows.

- [x] **Step 3: Make public media visibility consistent**

Use `FindPublicByID` in detail, cover, HLS manifest, and HLS segment paths so a private video cannot be fetched through a previously known URL. Change `Detail` to accept an optional viewer ID and fill state only when nonzero.

- [x] **Step 4: Run focused and integration tests**

Run:

`gofmt -w internal/video`

`go test ./internal/video -count=1`

`go test -tags=integration ./internal/video -run TestRepositoryListPublic -count=1`

Expected: PASS, including literal wildcard search.

- [x] **Step 5: Commit the searchable read model**

```bash
git add backend/internal/video
git commit -m "feat: add searchable public video discovery"
```

### Task 3: Author publishing and video management

**Files:**
- Modify: `backend/internal/video/dto.go`
- Modify: `backend/internal/video/repository.go`
- Modify: `backend/internal/video/service.go`
- Modify: `backend/internal/video/handler.go`
- Modify: `backend/internal/transcode/repository.go`
- Modify: `backend/internal/video/service_test.go`
- Modify: `backend/internal/video/handler_test.go`
- Modify: `backend/internal/transcode/service_test.go`

- [x] **Step 1: Write failing ownership, update, and deletion tests**

Test partial updates with pointer fields, trimmed title/description validation, public/private transitions, setting `published_at` only after a ready video becomes public, owner-only updates, idempotent logical deletion, and public URL disappearance after delete/private.

```go
type UpdateRequest struct {
    Title *string `json:"title"`
    Description *string `json:"description"`
    Visibility *string `json:"visibility"`
}
```

Run: `go test ./internal/video ./internal/transcode -run 'Test(Update|Delete|MarkSucceeded)' -count=1`

Expected: FAIL because author operations are missing.

- [x] **Step 2: Implement owner repository transactions**

Add `UpdateOwned(ctx, userID, videoID, patch)` and `DeleteOwned(ctx, userID, videoID)`. Update only explicitly supplied fields, preserve the first `published_at`, and use `WHERE id=? AND user_id=? AND status<>deleted`. Logical delete sets status to deleted and visibility private.

- [x] **Step 3: Update successful transcode publication metadata**

When the worker transitions a video to ready, set `published_at=COALESCE(published_at, UTC_TIMESTAMP(3))` only for public videos. Do this in the same transaction that marks the transcode job succeeded.

- [x] **Step 4: Add service and handlers, then run GREEN**

Expose `PATCH /users/me/videos/:id` and `DELETE /users/me/videos/:id`. Map an empty patch to `INVALID_PARAMETER`, ownership failures to `FORBIDDEN`, and already deleted/missing resources to `NOT_FOUND` without leaking another user's private metadata.

Run: `gofmt -w internal/video internal/transcode && go test ./internal/video ./internal/transcode -count=1`

Expected: PASS.

- [x] **Step 5: Commit author management**

```bash
git add backend/internal/video backend/internal/transcode
git commit -m "feat: add author video management"
```

### Task 4: Idempotent likes and favorites

**Files:**
- Create: `backend/internal/engagement/repository.go`
- Create: `backend/internal/engagement/service.go`
- Create: `backend/internal/engagement/dto.go`
- Create: `backend/internal/engagement/handler.go`
- Create: `backend/internal/engagement/service_test.go`
- Create: `backend/internal/engagement/handler_test.go`
- Create: `backend/internal/engagement/repository_integration_test.go`

- [x] **Step 1: Write failing service and repository tests**

Test first like/favorite, repeated enable, repeated disable, separate users, unavailable videos, missing authentication, and concurrent inserts. Assert relationship and counter change together and counts never become negative.

```go
type RelationState struct {
    Active bool `json:"active"`
    Count uint64 `json:"count"`
}
```

Run: `go test ./internal/engagement -run 'Test(Like|Favorite)' -count=1`

Expected: FAIL because the package behavior is not implemented.

- [x] **Step 2: Implement transactional relation changes**

Inside one GORM transaction, confirm the video is public and ready, use `INSERT IGNORE` for enable or a keyed `DELETE` for disable, and change the matching `video_stats` counter only when `RowsAffected == 1`. Read and return final relationship state after the write.

- [x] **Step 3: Implement handlers and error mapping**

Add `Like`, `Unlike`, `Favorite`, and `Unfavorite`; return HTTP 200 with final state for both first and repeated calls. Return 401 without a user, 404 for an unavailable video, and 500 for unexpected storage errors.

- [x] **Step 4: Run GREEN and integration tests**

Run: `gofmt -w internal/engagement && go test ./internal/engagement -count=1`

Run: `go test -tags=integration ./internal/engagement -run TestRelationTransactions -count=1`

Expected: PASS.

- [x] **Step 5: Commit likes and favorites**

```bash
git add backend/internal/engagement
git commit -m "feat: add video likes and favorites"
```

### Task 5: Comments and watch history

**Files:**
- Modify: `backend/internal/engagement/repository.go`
- Modify: `backend/internal/engagement/service.go`
- Modify: `backend/internal/engagement/dto.go`
- Modify: `backend/internal/engagement/handler.go`
- Modify: `backend/internal/engagement/service_test.go`
- Modify: `backend/internal/engagement/handler_test.go`
- Modify: `backend/internal/engagement/repository_integration_test.go`

- [x] **Step 1: Write failing comment and history tests**

Cover comment length 1–500 runes, newest-first pagination, author projection, owner-only soft delete, repeated delete, comment counter changes, first watch incrementing views, repeated watch updating progress without incrementing views, progress bounds, and history ordered by `last_watched_at`.

```go
type WatchRequest struct {
    ProgressMS uint64 `json:"progress_ms"`
    DurationMS uint64 `json:"duration_ms"`
}
```

Run: `go test ./internal/engagement -run 'Test(Comment|Watch|History)' -count=1`

Expected: FAIL because these methods are missing.

- [x] **Step 2: Implement comment transactions**

Create comments and increment `comment_count` in one transaction. Soft-delete with `WHERE id=? AND user_id=? AND deleted_at IS NULL`; decrement with `GREATEST(comment_count - 1, 0)` only when one row changed. Public listing filters `deleted_at IS NULL` and joins user public fields.

- [x] **Step 3: Implement watch upsert and history list**

Use `INSERT IGNORE` to establish `(user_id, video_id)` and detect a first watch. Increment `view_count` only for that insert, then update progress, duration, and `last_watched_at`. Reject `progress_ms > duration_ms` when duration is nonzero. History joins only currently public ready videos.

- [x] **Step 4: Implement handlers and run GREEN**

Expose comment list/create/delete, watch reporting, favorites list, and history list. Run:

`gofmt -w internal/engagement && go test ./internal/engagement -count=1`

`go test -tags=integration ./internal/engagement -run 'Test(CommentTransaction|WatchUpsert)' -count=1`

Expected: PASS.

- [x] **Step 5: Commit comments and history**

```bash
git add backend/internal/engagement
git commit -m "feat: add comments and watch history"
```

### Task 6: User follow graph

**Files:**
- Create: `backend/internal/follow/repository.go`
- Create: `backend/internal/follow/service.go`
- Create: `backend/internal/follow/dto.go`
- Create: `backend/internal/follow/handler.go`
- Create: `backend/internal/follow/service_test.go`
- Create: `backend/internal/follow/handler_test.go`
- Create: `backend/internal/follow/repository_integration_test.go`

- [x] **Step 1: Write failing follow tests**

Cover follow/unfollow idempotency, self-follow rejection, nonexistent/disabled target users, authentication, pagination, and newest-first follow list.

Run: `go test ./internal/follow -count=1`

Expected: FAIL because the follow package has models only.

- [x] **Step 2: Implement repository and service**

Use the `(follower_id, followee_id)` primary key with `INSERT IGNORE` and keyed `DELETE`. Return the final boolean state and make self-follow a stable `ErrSelfFollow` validation error.

- [x] **Step 3: Implement handlers and run GREEN**

Expose `PUT/DELETE /users/:id/follow` and `GET /users/me/follows`. Map self-follow to 400, missing target to 404, and missing viewer to 401.

Run: `gofmt -w internal/follow && go test ./internal/follow -count=1`

Expected: PASS.

- [x] **Step 4: Commit follow graph**

```bash
git add backend/internal/follow
git commit -m "feat: add user follow graph"
```

### Task 7: Authentication context, routing, and backend regression

**Files:**
- Modify: `backend/internal/middleware/auth.go`
- Modify: `backend/internal/middleware/auth_test.go`
- Modify: `backend/internal/server/router.go`
- Modify: `backend/internal/response/response.go`
- Create: `backend/internal/server/community_routes_test.go`
- Modify: `backend/README.md`

- [x] **Step 1: Write failing optional-auth and route tests**

Verify missing Authorization continues anonymously, a valid bearer token sets `user_id`, an invalid token continues anonymously on public routes, and every new write route still uses mandatory `Auth`. Exercise each HTTP method so accidental method/path omissions fail.

Run: `go test ./internal/middleware ./internal/server -run 'Test(OptionalAuth|CommunityRoutes)' -count=1`

Expected: FAIL because `OptionalAuth` and new route wiring do not exist.

- [x] **Step 2: Add optional authentication and dependency wiring**

Implement `OptionalAuth(tm)` without producing a response: parse a valid bearer token and set `user_id`; otherwise call `Next()` as anonymous. Construct video, engagement, and follow repositories/services/handlers once in `NewRouter`, and register every route from the approved design.

- [x] **Step 3: Add stable response codes and documentation**

Add `COMMENT_FORBIDDEN`, `SELF_FOLLOW`, and any shared conflict codes actually returned by handlers. Document all request bodies, query parameters, response shapes, idempotency, anonymous-view rules, and migration commands in `backend/README.md`.

- [x] **Step 4: Run backend verification**

Run:

`gofmt -w internal/middleware internal/server internal/response`

`go test ./... -count=1`

`go vet ./...`

`go build ./cmd/api ./cmd/worker`

Expected: all commands exit 0.

- [x] **Step 5: Commit backend route integration**

```bash
git add backend/internal/middleware backend/internal/server backend/internal/response backend/README.md
git commit -m "feat: expose community interaction APIs"
```

### Task 8: Frontend API clients and proxy allowlist

**Files:**
- Modify: `frontend/src/video.js`
- Create: `frontend/src/community.js`
- Modify: `frontend/src/auth.js`
- Modify: `frontend/server.mjs`
- Modify: `frontend/tests/video.test.mjs`
- Create: `frontend/tests/community.test.mjs`
- Modify: `frontend/tests/server.test.mjs`

- [x] **Step 1: Write failing client and proxy tests**

Assert exact URLs and methods for search/sort, update/delete, like/favorite, comment, watch, follow, favorites/history/follows, and public requests without a session. Assert the proxy allows only the designed methods, forwards query strings and bearer tokens, accepts JSON bodies for POST/PUT/PATCH/DELETE, and rejects unknown routes.

```js
await video.listVideos({ query: '猫', sort: 'popular', page: 2, pageSize: 12 });
// /api/v1/videos?q=%E7%8C%AB&sort=popular&page=2&page_size=12
```

Run: `npm test -- --test-name-pattern="community|search|proxy"`

Expected: FAIL because the methods and route patterns do not exist.

- [x] **Step 2: Implement focused clients**

Keep upload/discovery/owner methods in `video.js`; add `community.js` for interactions and personal community lists. Add a public request method to `auth.js` that shares response/error parsing but does not require or send a bearer token.

- [x] **Step 3: Extend the strict proxy**

Add `/src/community.js` to static files. Allow exact new route patterns and their explicit methods. Apply origin and JSON content-type checks to every body-carrying method, not only POST; forward the body unchanged and preserve the 32 KiB limit.

- [x] **Step 4: Run GREEN**

Run: `npm test`

Expected: all frontend tests pass.

- [x] **Step 5: Commit frontend clients**

```bash
git add frontend/src/auth.js frontend/src/video.js frontend/src/community.js frontend/server.mjs frontend/tests
git commit -m "feat: add community frontend clients"
```

### Task 9: Search, interaction, and personal-center interface

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/src/main.js`
- Create: `frontend/src/discover-view.js`
- Create: `frontend/src/detail-view.js`
- Create: `frontend/src/profile-view.js`
- Modify: `frontend/src/styles.css`
- Create: `frontend/tests/discover-view.test.mjs`
- Create: `frontend/tests/detail-view.test.mjs`
- Create: `frontend/tests/profile-view.test.mjs`
- Modify: `frontend/tests/video.test.mjs`
- Modify: `frontend/server.mjs`

- [x] **Step 1: Write failing pure-render and state tests**

Cover escaped rendering of search results/comments, empty results, search reset, retaining query/sort across pagination, optimistic like/favorite state with rollback on failure, comment validation, personal tabs, owner patch payloads, and watch-report throttling.

Run: `npm test -- --test-name-pattern="view|search|interaction|personal"`

Expected: FAIL because the view helpers and DOM controls are missing.

- [x] **Step 2: Add accessible page structure**

Add a labeled search form, sort select, result summary, stats in cards, detail action buttons with `aria-pressed`, comment form/list, author follow button, and a “我的” panel with 投稿/收藏/历史/关注 tabs. Add owner edit controls and an explicit delete confirmation dialog.

- [x] **Step 3: Implement UI state and events**

Keep server data as the source of truth. Search submission and sort changes reset page to 1; paging retains filters. Disable controls while writes are pending, update from the server's returned final state, and restore the previous state with a readable error on failure. Report watch once on `playing`, throttle progress reports, and make the final page-hide report best effort.

- [x] **Step 4: Style responsive and empty/error states**

Extend the existing visual language without changing the login screen. Ensure keyboard focus, 44 px action targets, mobile stacking, visible loading states, and comments that wrap untrusted long text.

- [x] **Step 5: Run frontend verification and commit**

Run: `node --check src/main.js; node --check src/discover-view.js; node --check src/detail-view.js; node --check src/profile-view.js; npm test`

Expected: syntax checks and all tests pass.

```bash
git add frontend/index.html frontend/src/main.js frontend/src/discover-view.js frontend/src/detail-view.js frontend/src/profile-view.js frontend/src/styles.css frontend/server.mjs frontend/tests
git commit -m "feat: add community interaction interface"
```

### Task 10: End-to-end acceptance and project documentation

**Files:**
- Create: `backend/scripts/e2e-community.ps1`
- Modify: `README.md`
- Modify: `backend/README.md`
- Modify: `frontend/README.md`
- Update: `docs/superpowers/plans/2026-09-14-stage4-community-interactions.md`

- [x] **Step 1: Write an API acceptance script**

The script creates two uniquely named users, uploads or reuses a ready test video for user A, searches for it as user B, then watches, likes, favorites, comments, follows, lists personal data, cancels every reversible relationship, and verifies final counters. It must fail on any unexpected status or response value and clean up only its own generated data.

- [x] **Step 2: Apply migrations and run all automated checks**

Run:

`go run ./cmd/api migrate up`

`go test ./... -count=1`

`go test -tags=integration ./... -count=1`

`go vet ./...`

`go build ./cmd/api ./cmd/worker`

`npm test`

Expected: migrations apply once, repeat as a no-op, and every check exits 0.

- [ ] **Step 3: Run browser acceptance**

Start API, worker, and frontend; use two test accounts to verify search, playback, like, favorite, comment, follow, history, owner edit/visibility/delete, mobile layout, logout, and anonymous public playback. Inspect browser console and API/worker logs for uncaught errors.

> 未完成：当前环境无法构建 api/worker 镜像（Docker Hub 经代理不可达），本机也没有 ffmpeg 和可复用的 worker 容器，因此 `scripts/e2e-community.ps1` 与浏览器验收都缺少可运行的服务端。脚本已写好并做了语法校验，待有可用环境时执行。

- [x] **Step 4: Update runbooks and architecture documentation**

Document new tables and fields, endpoint examples, how idempotency and counters work, how search escapes wildcard characters, how to run the acceptance script, and the explicit boundary for future Redis/Kafka search/statistics scaling.

- [x] **Step 5: Final review and commit**

Run `git diff --check`, review for secrets, SQL injection, authorization gaps, counter drift, stale frontend state, and accidental MinIO object-key exposure. Mark every completed checkbox in this plan.

```bash
git add README.md backend/README.md backend/scripts/e2e-community.ps1 frontend/README.md docs/superpowers/plans/2026-09-14-stage4-community-interactions.md
git commit -m "docs: complete stage 4 community workflow"
```
