# Stage 3 Video Processing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use executing-plans and complete each checkbox in order.

**Goal:** Convert verified MP4 uploads into durable asynchronous Kafka jobs that produce private MinIO HLS renditions and covers, while exposing safe playback APIs.

**Architecture:** `complete` commits a video state transition, transcode job, and outbox event in one MySQL transaction. An API-hosted dispatcher publishes outbox events to Kafka; a separate worker claims jobs idempotently, runs ffprobe/ffmpeg, uploads deterministic outputs, and atomically marks videos ready. HLS manifests are rewritten by the API and media objects redirect to short-lived MinIO URLs.

**Tech Stack:** Go, Gin, GORM, Goose, MySQL 8, MinIO, franz-go Kafka client, FFmpeg/FFprobe, Docker Compose.

---

### Task 1: Schema and domain lifecycle

**Files:**
- Create: `backend/migrations/000003_add_video_processing.sql`
- Modify: `backend/internal/video/model.go`
- Modify: `backend/internal/video/repository.go`
- Test: `backend/internal/video/service_test.go`

- [x] Add `processing=5` without renumbering existing states.
- [x] Add processing metadata columns, `video_transcode_jobs`, and `outbox_events` with rollback SQL and scan indexes.
- [x] Add conditional repository transitions and transactionally enqueue complete requests.
- [x] Verify duplicate `complete` calls do not create duplicate jobs.

### Task 2: Reliable Kafka delivery

**Files:**
- Create: `backend/internal/messaging/kafka.go`
- Create: `backend/internal/outbox/model.go`
- Create: `backend/internal/outbox/repository.go`
- Create: `backend/internal/outbox/dispatcher.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/cmd/api/main.go`
- Tests: matching `_test.go` files

- [x] Define versioned transcode event JSON.
- [x] Poll due outbox rows safely, publish with an event key, and conditionally record success/failure.
- [x] Start and stop the dispatcher with the API lifecycle.
- [x] Verify Kafka downtime leaves durable pending rows.

### Task 3: FFmpeg worker

**Files:**
- Create: `backend/cmd/worker/main.go`
- Create: `backend/internal/transcode/probe.go`
- Create: `backend/internal/transcode/profiles.go`
- Create: `backend/internal/transcode/runner.go`
- Create: `backend/internal/transcode/service.go`
- Extend: `backend/internal/storage/minio.go`
- Tests: matching `_test.go` files

- [x] Claim jobs with conditional updates and ignore completed duplicates.
- [x] Download source, parse ffprobe JSON, generate cover and HLS profiles without upscaling.
- [x] Upload deterministic output keys with correct content types.
- [x] Persist progress, retry with exponential backoff, and mark terminal failures.
- [x] Clean temporary files and stop on context cancellation.

### Task 4: HLS and owner APIs

**Files:**
- Modify: `backend/internal/video/dto.go`
- Modify: `backend/internal/video/service.go`
- Modify: `backend/internal/video/handler.go`
- Modify: `backend/internal/server/router.go`
- Create: `backend/internal/video/hls.go`
- Tests: matching `_test.go` files

- [x] Return processing metadata without exposing internal object keys.
- [x] Add owner detail polling endpoint.
- [x] Rewrite private HLS manifests with strict path containment.
- [x] Redirect segments and covers to short-lived presigned URLs.
- [x] Keep public list/detail restricted to ready videos.

### Task 5: Containerized development stack

**Files:**
- Create: `backend/Dockerfile`
- Modify: `backend/deploy/docker-compose.yml`
- Modify: `backend/.env.example`

- [x] Add pinned Kafka KRaft service and persistent volume.
- [x] Add migrate, API, and FFmpeg worker containers with health/dependency gates.
- [x] Preserve existing MySQL/MinIO volumes and host ports.
- [ ] Verify `docker compose config` (the current Docker named pipe/approval limit blocked this command in the agent session).

### Task 6: Documentation and acceptance

**Files:**
- Create: `backend/docs/stage3-frontend-contract.md`
- Create: `backend/scripts/e2e-transcode.ps1`
- Modify: `backend/README.md`

- [x] Document request/response contracts, status semantics, architecture, and runbooks.
- [ ] Generate a tiny MP4 and test upload, processing, HLS, cover, and segment access (FFmpeg is absent on the host and Docker named pipe access was blocked).
- [x] Run gofmt, unit/integration tests, vet, builds, and migration checks.
- [x] Review security, concurrency, retries, cleanup, and secret handling before completion.
