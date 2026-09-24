# Stage 5 T11 failure drills

Run these only against an explicitly named isolated smoke Compose project and synthetic test data. Do not stop or mutate the ordinary development services, use the business database, or run database-wide cleanup. Restore every stopped service in a `finally`/operator recovery step. Keep logs free of credentials, refresh cookies, and bearer tokens.

Compose target used by the current local smoke stack:

```powershell
$project = 'stage5-smoke-20260924'
$base = @('-p', $project, '-f', 'backend/deploy/docker-compose.yml', '-f', 'backend/deploy/docker-compose.isolated-smoke.yml')
```

## Drill matrix

| Failure | Injection and observation | Recovery evidence | Status |
|---|---|---|---|
| Kafka unavailable during transcode enqueue | `-DrillKafkaOutage` stops only isolated Kafka, uploads/completes a synthetic video, and reads that video's outbox row while the broker is down. Outbox publish attempts are bounded by `KAFKA_PUBLISH_TIMEOUT_MS` (default 5000 ms). | Observed the row pending (`status=1`), with non-empty `last_error` and `attempts=2` while Kafka was down. After Kafka start, the same row became published (`status=2`), `attempts=4`, `last_error` was cleared, and the video reached ready. `attempts` is a dispatcher-claim counter, not a broker-delivery count. | PASS: bounded failure, retry claim, and recovery |
| Duplicate Kafka event | `-DrillDuplicateKafkaEvent` reads the original payload for a ready synthetic video and writes it again to the isolated topic with its original key and `event_id`. | Worker emitted a second handled log for that `event_id`; the job remained succeeded with attempts=1 and the database retained one video row. The first script attempt had a trailing blank line that made the CLI exit nonzero after sending the record; removed the extra newline and reran successfully. | PASS on corrected rerun |
| Worker exits after claiming work | `-DrillWorkerExit` creates a 60-second synthetic 720p fixture, waits for `processing_progress >= 20`, sends KILL only to isolated Worker, then explicitly starts that service again with the same `WORKER_ID`. | The video reached ready after redelivery; the same transcode job was succeeded with attempts=2 and exactly one video row. `docker compose kill` is a manual stop and does not activate `restart: unless-stopped`; the script now uses explicit `start worker`. | PASS |
| Redis unavailable | `-DrillRedisOutage` stops only isolated Redis and requests categories and day ranking through the isolated frontend/API. | Received 6 categories from the MySQL-backed loader and a successful day ranking fallback; Redis was started and returned healthy. During the later T12 measurement, Redis reads also stayed HTTP 200 but took about 2 s at this small dataset; the periodic ranking-rebuild container logged Redis DNS/lock errors and restarted while Redis was down, then rebuilt day/week after recovery. Rate-limit fallback was not separately asserted in this drill. | PASS for category/ranking reads; latency and builder restart documented |
| Hot-key expiry and ranking rebuild | `-DrillCacheExpiry` changes only `video_share:cache:categories:v1` to a one-second TTL, waits for expiry, reads categories, and rebuilds day/week rankings. | The exact category key expired and reloaded with TTL 314 s. Day snapshot/metadata TTLs were 179/178 s; week TTLs 178/177 s. Rebuild used MySQL metrics and retained finite Redis TTLs. | PASS |
| Database transaction failure | Ran real MySQL moderation integration tests against the isolated smoke MySQL. `TestModerationReportLifecycleAndVisibility` submits a deliberately invalid report association to a moderation action and asserts the transactional visibility state and action count remain unchanged. No service or business DB was stopped. | `go test -tags integration ./internal/moderation -count=1` passed all 3 tests. Test helper created unique databases and removed them; a follow-up query found no `video_share_test_%` schemas. | PASS (transaction rollback) |
| Two-admin concurrent handling | The same real MySQL integration run races two admin assignments for one report and exercises report/target lock-order races. | Exactly one concurrent assignment succeeded and the other returned the expected state conflict; the wider lifecycle test verified committed audit/notification state and no lock-order timeout. | PASS |

## Safe command patterns

Always inspect the resolved project before injecting a fault:

```powershell
docker compose @base ps
docker compose @base stop kafka
try {
  # Run the isolated synthetic probe and collect only redacted logs/observations.
} finally {
  docker compose @base start kafka
  docker compose @base ps kafka
}
```

The executable API/browser entry point and fault switches are:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1 -DrillKafkaOutage -SkipBrowser
powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1 -DrillRedisOutage -DrillCacheExpiry -SkipBrowser
powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1 -DrillDuplicateKafkaEvent -SkipBrowser
powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1 -DrillWorkerExit -SkipBrowser
```

Use equivalent service-scoped commands for `redis` or `worker`; after a manual Worker kill, use `docker compose @base start worker`. Do not use `up -d worker` as a recovery shortcut: Compose may reconcile/recreate dependencies if the launch-shell variables differ. Start/reconcile the isolated stack only from the same PowerShell session that holds its generated `STAGE5_SMOKE_*` values, or regenerate a complete new isolated project. Never use `docker compose down -v`, `docker volume prune`, or broad `docker rm`. The `mysql` service is the isolated project database only. The smoke-only MySQL healthcheck probes the application database identity rather than depending on an ephemeral root password.

## Evidence log

## 2026-09-24 evidence log

- Branch `main`, base commit `3cee3f4964fe224bb37cbd77b90be74c824e12b7`, dirty working tree preserved; isolated Compose project `stage5-smoke-20260924` only.
- Main API + real-browser E2E: exit 0; 1 Playwright test passed. Existing `e2e-transcode.ps1` and `e2e-community.ps1` each passed under Windows PowerShell 5.1.
- Kafka stop/recovery after adding bounded publisher context: while the broker was down, the pending row reached `attempts=2` with a non-empty `last_error`; after restart it published at `attempts=4`, cleared `last_error`, and the Worker completed the video. The rerun command was `powershell.exe -NoProfile -ExecutionPolicy Bypass -File backend/scripts/e2e-stage5.ps1 -DrillKafkaOutage -SkipBrowser` (exit 0); browser was intentionally skipped because this is a fault drill. The normal full E2E run separately passed the browser test.
- Backend and frontend production images were rebuilt from the current working tree; the migration command reported `applied:0`. API, Worker, ranking rebuild, and frontend were recreated only in the isolated project.
- Both production and isolated Nginx configurations passed `nginx -t`. In a DNS-rotation probe, the isolated API IP changed from `172.20.0.7` to `172.20.0.10` while the frontend stayed running; after the 5-second resolver cache interval, the frontend's same-origin categories proxy returned HTTP 200.
- Nginx returned HTTP 413 for a 33,000-byte API body; MinIO OPTIONS preflight allowed only the configured isolated app Origin; the frontend returned its CSP header. The Stage5 API script asserts refresh-cookie `HttpOnly` and `SameSite=Lax` attributes, and the real-browser flow passed.
- `go test -race ./...` passed after the bounded Outbox publish change.
- Duplicate replay: corrected run exit 0; original `event_id` appeared in a second Worker handled log; one succeeded job and one video row.
- Worker exit: exit 0; job was claimed before KILL at 20%, Worker was explicitly started, event was redelivered, and the job reached succeeded with attempts=2.
- Redis/cache: categories and ranking returned while Redis was unavailable; Redis health recovered; category TTL expiry/reload and day/week rebuild TTLs observed.
- MySQL moderation: `go test -tags integration ./internal/moderation -count=1` exit 0 with `REQUIRE_INTEGRATION_TESTS=true`; transaction rollback and two-admin race tests ran against isolated test schemas. Temporary integration account removed.
- End state: API, frontend, Kafka, MySQL, MinIO, Redis healthy; Worker running. Synthetic videos created by these scripts were soft-deleted. No test database schemas remained. Synthetic identities/audit data remain in the isolated Compose volume by design.
- Recovery note: one `docker compose up -d worker` from a shell that did not hold the launch-generated `STAGE5_SMOKE_*` values reconciled/recreated only the isolated MySQL and MinIO containers against their existing project volumes. No volume was removed. This turn rehydrated the isolated runtime values from container metadata into process-local variables without printing them, ran the no-op migration (`applied:0`), and recreated only application containers. A future test run needing `CREATE/DROP DATABASE` should use a fresh isolated stack with its launch variables retained or rehydrate the isolated DB admin credential only in the process environment. The ordinary development/business database was never targeted.
