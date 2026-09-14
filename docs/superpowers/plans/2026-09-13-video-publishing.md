# Video Publishing MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an end-to-end MP4 publishing and playback flow to the existing authenticated video sharing application.

**Architecture:** MySQL owns video metadata and state, while MinIO owns binary objects. The Go API issues presigned URLs and enforces ownership/state rules; the existing dependency-free frontend uses its same-origin proxy for JSON APIs and uploads directly to MinIO.

**Tech Stack:** Go, Gin, GORM, Goose, MinIO Go SDK, MySQL, native HTML/CSS/JavaScript, Node HTTP server and node:test.

---

### Task 1: Configuration and database migration

**Files:** `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/.env.example`, `backend/deploy/docker-compose.yml`, `backend/migrations/000002_create_videos.sql`

- [x] Add storage endpoint, credentials, bucket, TLS and URL expiry configuration with validation tests.
- [x] Add a reversible `videos` migration with foreign key and list indexes.
- [x] Add MinIO to local Docker Compose and document development defaults without committing real secrets.
- [x] Run `go test ./internal/config ./internal/database`.

### Task 2: Object storage boundary

**Files:** `backend/internal/storage/storage.go`, `backend/internal/storage/minio.go`, `backend/internal/storage/minio_test.go`, `backend/go.mod`, `backend/go.sum`

- [x] Define a small storage interface for presign, stat and bucket initialization.
- [x] Implement it with the official MinIO Go SDK.
- [x] Unit-test validation and client behavior without a live MinIO dependency.
- [x] Run `go test ./internal/storage`.

### Task 3: Video domain

**Files:** `backend/internal/video/model.go`, `dto.go`, `repository.go`, `service.go`, `service_test.go`, `handler.go`, `handler_test.go`

- [x] Define video statuses, external DTOs and validation rules.
- [x] Implement repository create, ownership lookup, ready transition, public pagination and owner listing.
- [x] Implement create, complete, list and detail services with ownership and object validation.
- [x] Implement Gin handlers and stable HTTP error mapping.
- [x] Run `go test ./internal/video`.

### Task 4: Wire backend routes

**Files:** `backend/cmd/api/main.go`, `backend/internal/server/router.go`, `backend/internal/response/response.go`, server tests

- [x] Initialize the MinIO storage client and bucket during application startup.
- [x] Inject video dependencies into the router.
- [x] Register authenticated create/complete/mine routes and public list/detail routes.
- [x] Run `go test ./...` and `go vet ./...`.

### Task 5: Frontend API client and proxy

**Files:** `frontend/src/video.js`, `frontend/src/auth.js`, `frontend/server.mjs`, `frontend/tests/video.test.mjs`, `frontend/tests/server.test.mjs`

- [x] Expose authenticated request access without persisting the JWT outside the in-memory client.
- [x] Add create, direct upload, complete, list, detail and mine client functions.
- [x] Extend the allowlisted proxy to dynamic video paths and methods while retaining origin, size and header controls.
- [x] Add client and proxy tests and run `npm.cmd test`.

### Task 6: Frontend application screens

**Files:** `frontend/index.html`, `frontend/src/main.js`, `frontend/src/styles.css`, `frontend/README.md`

- [x] Add the authenticated application shell and navigation.
- [x] Build discovery cards, empty/loading/error states and video detail player.
- [x] Build the upload form with file validation and three-stage progress.
- [x] Build the current user's submission list with status labels.
- [x] Preserve keyboard access, responsive layout and reduced-motion behavior.

### Task 7: Documentation and full verification

**Files:** `backend/README.md`, `frontend/README.md`

- [x] Document MinIO startup, configuration, migration and manual end-to-end steps.
- [x] Run `gofmt`, `go test ./...`, `go vet ./...`, `npm.cmd test` and syntax checks.
- [x] Inspect `git diff` for secrets, generated binaries and unrelated changes.
