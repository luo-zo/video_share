param(
    [string]$BaseUrl = "http://127.0.0.1:15173",
    [string]$ComposeProject = "stage5-smoke-20260924",
    [int]$TimeoutSeconds = 240,
    [switch]$DrillKafkaOutage,
    [switch]$DrillRedisOutage,
    [switch]$DrillCacheExpiry,
    [switch]$DrillDuplicateKafkaEvent,
    [switch]$DrillWorkerExit,
    [switch]$SkipBrowser
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$BaseUrl = $BaseUrl.TrimEnd("/")
$baseUri = [Uri]$BaseUrl
if ($baseUri.Scheme -ne "http" -or $baseUri.Host -notin @("127.0.0.1", "localhost", "::1")) {
    throw "Stage 5 E2E is restricted to a local HTTP endpoint. Use the isolated smoke stack."
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$composeBase = @(
    "-p", $ComposeProject,
    "-f", (Join-Path $PSScriptRoot "../deploy/docker-compose.yml"),
    "-f", (Join-Path $PSScriptRoot "../deploy/docker-compose.isolated-smoke.yml")
)
$tempVideo = Join-Path ([IO.Path]::GetTempPath()) ("video-share-stage5-{0}.mp4" -f [guid]::NewGuid().ToString("N"))
$author = $null
$viewer = $null
$admin = $null
$videoID = [uint64]0
$videoHeaders = $null
$browserEnvNames = @("STAGE5_E2E_BASE_URL", "STAGE5_E2E_USERNAME", "STAGE5_E2E_PASSWORD", "STAGE5_E2E_VIDEO_ID", "STAGE5_E2E_VIDEO_TITLE")
$priorBrowserEnv = @{}
foreach ($name in $browserEnvNames) { $priorBrowserEnv[$name] = [Environment]::GetEnvironmentVariable($name, "Process") }

function Assert-True($Condition, [string]$Label) {
    if (-not $Condition) { throw "assertion failed: $Label" }
}

function Assert-Equal($Actual, $Expected, [string]$Label) {
    if ($Actual -ne $Expected) {
        throw ("assertion failed: {0} (got '{1}', want '{2}')" -f $Label, $Actual, $Expected)
    }
}

function Resolve-StageUrl([string]$Value) {
    if ([Uri]::IsWellFormedUriString($Value, [UriKind]::Absolute)) { return $Value }
    return $script:BaseUrl + "/" + $Value.TrimStart("/")
}

function Invoke-StageCompose([string[]]$Arguments) {
    $all = @("compose") + $script:composeBase + $Arguments
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw ("docker compose {0} failed: {1}" -f ($Arguments -join " "), ($output -join "`n"))
    }
    return ,$output
}

function Invoke-StageComposeWithInput([string[]]$Arguments, [string]$InputText) {
    $all = @("compose") + $script:composeBase + $Arguments
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = ($InputText + "`n") | & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw ("docker compose {0} failed: {1}" -f ($Arguments -join " "), ($output -join "`n"))
    }
    return ,$output
}

function Reset-StageRegistrationBucket {
    $frontendIDs = Invoke-StageCompose @("ps", "-q", "frontend")
    $frontendID = [string](@($frontendIDs | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }) | Select-Object -First 1)
    if ([string]::IsNullOrWhiteSpace($frontendID)) { throw "isolated frontend container is not running" }
    $frontendIP = (& docker inspect "--format={{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" $frontendID 2>&1)
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace([string]$frontendIP)) { throw "could not resolve isolated frontend IP for its registration rate bucket" }
    $hash = [Security.Cryptography.SHA256]::Create()
    try {
        $digest = [BitConverter]::ToString($hash.ComputeHash([Text.Encoding]::UTF8.GetBytes(([string]$frontendIP).Trim()))).Replace("-", "").ToLowerInvariant()
    } finally { $hash.Dispose() }
    $key = "video_share:rate:register:$digest"
    $deleted = Invoke-StageCompose @("exec", "-T", "redis", "sh", "-c", 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli --no-auth-warning DEL "$1"', "stage5-reset-register-bucket", $key)
    $result = (@($deleted) -join "`n").Trim()
    if ($result -notmatch "^[01]$") { throw "isolated registration bucket reset returned an unexpected result" }
    Write-Host "      reset only the isolated frontend's short-lived registration quota key"
}

function Get-StageOutboxState([uint64]$VideoID) {
    $query = "SELECT status, attempts, COALESCE(last_error, '<none>') FROM outbox_events WHERE event_type=0x766964656f2e7472616e73636f64652e726571756573746564 AND event_key=$VideoID ORDER BY id DESC LIMIT 1"
    $command = 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" -D "$MYSQL_DATABASE"'
    $result = Invoke-StageComposeWithInput @("exec", "-T", "mysql", "sh", "-c", $command) $query
    $line = [string](@($result | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }) | Select-Object -Last 1)
    $values = $line.Trim() -split "`t"
    if ($values.Count -ne 3) { throw "could not read the isolated outbox state for video_id=$VideoID" }
    $lastError = if ($values[2] -eq '<none>') { '' } else { [string]$values[2] }
    return @{ status = [int]$values[0]; attempts = [int]$values[1]; lastError = $lastError }
}

function Get-StageOutboxPayload([uint64]$VideoID) {
    $query = "SELECT CAST(payload AS CHAR) FROM outbox_events WHERE event_type=0x766964656f2e7472616e73636f64652e726571756573746564 AND event_key=$VideoID ORDER BY id DESC LIMIT 1"
    $command = 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" -D "$MYSQL_DATABASE"'
    $result = Invoke-StageComposeWithInput @("exec", "-T", "mysql", "sh", "-c", $command) $query
    $payload = [string](@($result | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }) | Select-Object -Last 1)
    if ($payload -notmatch '^\{.*\}$') { throw "could not read the isolated outbox payload for video_id=$VideoID" }
    return $payload.Trim()
}

function Get-StageTranscodeJobState([uint64]$VideoID) {
    $query = "SELECT status, attempts, (SELECT COUNT(*) FROM videos WHERE id=$VideoID) FROM video_transcode_jobs WHERE video_id=$VideoID"
    $command = 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" -D "$MYSQL_DATABASE"'
    $result = Invoke-StageComposeWithInput @("exec", "-T", "mysql", "sh", "-c", $command) $query
    $line = [string](@($result | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }) | Select-Object -Last 1)
    $values = $line.Trim() -split "`t"
    if ($values.Count -ne 3) { throw "could not read the isolated transcode job state for video_id=$VideoID" }
    return @{ status = [int]$values[0]; attempts = [int]$values[1]; videoCount = [int]$values[2] }
}

function Get-StageWorkerStartedAt([string]$ContainerID) {
    $result = & docker inspect "--format={{.State.StartedAt}}" $ContainerID 2>&1
    if ($LASTEXITCODE -ne 0) { throw "could not inspect only the isolated worker container: $($result -join ' ')" }
    return [DateTime]::Parse([string]$result).ToUniversalTime()
}

function Publish-StageDuplicateKafkaEvent([uint64]$VideoID, [string]$Payload) {
    $record = "$VideoID`:$Payload"
    $all = @("compose") + $script:composeBase + @(
        "exec", "-T", "kafka", "/opt/kafka/bin/kafka-console-producer.sh",
        "--bootstrap-server", "kafka:29092", "--topic", "video.transcode.requested",
        "--property", "parse.key=true", "--property", "key.separator=:"
    )
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = $record | & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) { throw ("could not re-publish the synthetic Kafka event: {0}" -f ($output -join "`n")) }
}

function Wait-StageKafkaHealthy {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        $lines = Invoke-StageCompose @("ps", "kafka")
        if ((@($lines) -join "`n") -match "healthy") { return }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "the isolated Kafka service did not become healthy after restart"
}

function Wait-StageRedisHealthy {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        $lines = Invoke-StageCompose @("ps", "redis")
        if ((@($lines) -join "`n") -match "healthy") { return }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "the isolated Redis service did not become healthy after restart"
}

function Invoke-StageRedisCommand([string]$Command) {
    $shellCommand = 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli --no-auth-warning ' + $Command
    $result = Invoke-StageCompose @("exec", "-T", "redis", "sh", "-c", $shellCommand)
    return (@($result) -join "`n").Trim()
}

function Invoke-StageRedisOutageDrill {
    Write-Host "      stop only isolated Redis; verify category and ranking reads degrade to MySQL"
    try {
        Invoke-StageCompose @("stop", "redis") | Out-Null
        $categories = Invoke-StageApi -Method Get -Path "/api/v1/categories" -TimeoutSec 15
        Assert-True (@($categories.data.items).Count -gt 0) "category list falls back to MySQL while Redis is unavailable"
        $ranking = Invoke-StageApi -Method Get -Path "/api/v1/videos/ranking?window=day&page=1&page_size=5" -TimeoutSec 15
        Assert-Equal $ranking.data.window "day" "ranking falls back to its bounded MySQL path while Redis is unavailable"
        Write-Host ("      fallback responses: categories={0}, ranking_window={1}" -f @($categories.data.items).Count, $ranking.data.window)
    } finally {
        Invoke-StageCompose @("start", "redis") | Out-Null
        Wait-StageRedisHealthy
    }
}

function Invoke-StageCacheExpiryDrill {
    $categoryKey = "video_share:cache:categories:v1"
    $categoryTTL = [int](Invoke-StageRedisCommand "TTL $categoryKey")
    Assert-True ($categoryTTL -ge 0) "category cache is present before expiry"
    $expired = Invoke-StageRedisCommand "EXPIRE $categoryKey 1"
    Assert-Equal $expired "1" "the exact category cache key accepts a one-second test TTL"
    $expiryDeadline = [DateTime]::UtcNow.AddSeconds(10)
    do {
        $exists = Invoke-StageRedisCommand "EXISTS $categoryKey"
        if ($exists -eq "0") { break }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $expiryDeadline)
    Assert-Equal $exists "0" "the isolated category hot key expires"
    $categories = Invoke-StageApi -Method Get -Path "/api/v1/categories"
    Assert-True (@($categories.data.items).Count -gt 0) "category cache reloads from MySQL after expiry"
    $reloadedTTL = [int](Invoke-StageRedisCommand "TTL $categoryKey")
    Assert-True ($reloadedTTL -gt 0) "category cache is repopulated with a finite TTL"

    foreach ($window in @("day", "week")) {
        Invoke-StageCompose @("exec", "-T", "-e", "RANKING_REBUILD_ONCE=1", "ranking-rebuild", "/app/ranking-rebuild", $window) | Out-Null
    }
    $dateKey = [DateTimeOffset]::UtcNow.ToOffset([TimeSpan]::FromHours(8)).ToString("yyyy-MM-dd")
    foreach ($window in @("day", "week")) {
        $snapshotTTL = [int](Invoke-StageRedisCommand "TTL video_share:rank:$window`:$dateKey")
        $metadataTTL = [int](Invoke-StageRedisCommand "TTL video_share:rank:$window`:$dateKey`:generated_at")
        Assert-True ($snapshotTTL -gt 0 -and $metadataTTL -gt 0) "$window ranking snapshot and metadata are rebuilt with finite TTLs"
        Write-Host ("      {0} ranking snapshot TTL={1}s metadata TTL={2}s" -f $window, $snapshotTTL, $metadataTTL)
    }
    Write-Host ("      exact category cache key expired and reloaded (new TTL={0}s)" -f $reloadedTTL)
}

function Invoke-StageApi {
    param(
        [string]$Method,
        [string]$Path,
        [Microsoft.PowerShell.Commands.WebRequestSession]$Session,
        [string]$AccessToken,
        [string]$RequestID,
        $Body,
        [int[]]$ExpectStatus = @(200),
        [int]$TimeoutSec = 30
    )
    $headers = @{ Origin = $script:BaseUrl }
    if ($AccessToken) { $headers.Authorization = "Bearer $AccessToken" }
    if ($RequestID) { $headers["Idempotency-Key"] = $RequestID }
    if ($null -ne $Session) {
        $csrf = $Session.Cookies.GetCookies([Uri]$script:BaseUrl) | Where-Object { $_.Name -eq "video_share_csrf" } | Select-Object -First 1
        if ($null -ne $csrf -and -not [string]::IsNullOrWhiteSpace($csrf.Value)) { $headers["X-CSRF-Token"] = $csrf.Value }
    }
    $params = @{
        Method = $Method
        Uri = (Resolve-StageUrl $Path)
        Headers = $headers
        UseBasicParsing = $true
        TimeoutSec = $TimeoutSec
    }
    if ($null -ne $Session) { $params.WebSession = $Session }
    if ($null -ne $Body) {
        $params.ContentType = "application/json; charset=utf-8"
        $params.Body = ($Body | ConvertTo-Json -Depth 12 -Compress)
    }
    if ($PSVersionTable.PSVersion.Major -ge 6) { $params.SkipHttpErrorCheck = $true }

    $status = 0
    $content = ""
    $responseHeaders = @{}
    try {
        $response = Invoke-WebRequest @params
        $status = [int]$response.StatusCode
        $content = $response.Content
        $responseHeaders = $response.Headers
        if ($content -is [byte[]]) { $content = [Text.Encoding]::UTF8.GetString($content) }
    } catch {
        $webResponse = $_.Exception.Response
        if ($null -eq $webResponse) { throw }
        $status = [int]$webResponse.StatusCode
        $responseHeaders = $webResponse.Headers
        if ($null -ne $_.ErrorDetails -and -not [string]::IsNullOrWhiteSpace($_.ErrorDetails.Message)) {
            $content = $_.ErrorDetails.Message
        } else {
            $reader = New-Object System.IO.StreamReader($webResponse.GetResponseStream())
            $content = $reader.ReadToEnd()
            $reader.Dispose()
        }
    }
    if ($ExpectStatus -notcontains $status) {
        throw ("{0} {1} returned {2}, expected {3}. Body: {4}" -f $Method, $Path, $status, ($ExpectStatus -join "/"), $content)
    }
    $parsed = $null
    if (-not [string]::IsNullOrWhiteSpace($content)) {
        try { $parsed = $content | ConvertFrom-Json } catch { $parsed = $null }
    }
    return @{ status = $status; body = $parsed; data = $(if ($null -ne $parsed) { $parsed.data } else { $null }); raw = $content; headers = $responseHeaders }
}

function New-StageIdentity([string]$Prefix, [string]$Nickname) {
    $suffix = [DateTime]::UtcNow.ToString("yyMMddHHmmss") + ([guid]::NewGuid().ToString("N").Substring(0, 4))
    $username = "{0}_{1}" -f $Prefix, $suffix
    $password = [guid]::NewGuid().ToString("N") + "Aa1!"
    $register = Invoke-StageApi -Method Post -Path "/api/v1/auth/register" -Body @{ username = $username; password = $password; nickname = $Nickname } -ExpectStatus @(201)
    Assert-True ($register.data.user -or $register.data.id) "registration returns a created identity"
    return @{ username = $username; password = $password; nickname = $Nickname; id = [uint64]$(if ($register.data.user) { $register.data.user.id } else { $register.data.id }) }
}

function Connect-StageIdentity($Identity) {
    $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $csrf = Invoke-StageApi -Method Get -Path "/api/v1/auth/csrf" -Session $session
    Assert-True ($csrf.data.csrf_token -is [string] -and $csrf.data.csrf_token.Length -gt 0) "CSRF token endpoint sets a token"
    $login = Invoke-StageApi -Method Post -Path "/api/v1/auth/login" -Session $session -Body @{ username = $Identity.username; password = $Identity.password }
    $setCookieHeaders = @($login.headers["Set-Cookie"])
    $refreshSetCookie = [string](@($setCookieHeaders | Where-Object { $_ -like "video_share_refresh=*" }) | Select-Object -First 1)
    Assert-True ($refreshSetCookie -match "(?i)HttpOnly") "refresh Set-Cookie response is HttpOnly"
    Assert-True ($refreshSetCookie -match "(?i)SameSite=Lax") "refresh Set-Cookie response is SameSite=Lax"
    $token = [string]$login.data.access_token
    Assert-True ($token.Length -gt 0) "login returns an access token"
    $Identity.accessToken = $token
    $Identity.session = $session
    $Identity.user = $login.data.user
    return $Identity
}

function Invoke-StageWorkerExitDrill($Owner, [uint64]$VideoID) {
    $claimDeadline = [DateTime]::UtcNow.AddSeconds($script:TimeoutSeconds)
    $detail = $null
    do {
        $detail = Invoke-StageApi -Method Get -Path "/api/v1/users/me/videos/$VideoID" -Session $Owner.session -AccessToken $Owner.accessToken
        if ($detail.data.status -eq "failed") { throw "transcode failed before Worker-exit drill: $($detail.data.processing_error)" }
        if ($detail.data.status -eq "processing" -and [int]$detail.data.processing_progress -ge 20) { break }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $claimDeadline)
    Assert-True ($null -ne $detail -and $detail.data.status -eq "processing" -and [int]$detail.data.processing_progress -ge 20) "Worker has claimed the job and reached the processing checkpoint before termination"

    $workerIDs = Invoke-StageCompose @("ps", "-q", "worker")
    $workerID = [string](@($workerIDs | Where-Object { -not [string]::IsNullOrWhiteSpace([string]$_) }) | Select-Object -First 1)
    if ([string]::IsNullOrWhiteSpace($workerID)) { throw "isolated Worker container is not running" }
    $startedAtBefore = Get-StageWorkerStartedAt $workerID
    Write-Host ("      force-exit only isolated Worker after job claim at progress={0}%" -f $detail.data.processing_progress)
    try {
        Invoke-StageCompose @("kill", "--signal", "KILL", "worker") | Out-Null
        # A manual Compose kill does not trigger Docker's restart policy. Start
        # this exact service explicitly, without recreating its dependencies.
        Invoke-StageCompose @("start", "worker") | Out-Null
        $recoveryDeadline = [DateTime]::UtcNow.AddSeconds($script:TimeoutSeconds)
        $startedAt = $startedAtBefore
        do {
            Start-Sleep -Milliseconds 500
            $startedAt = Get-StageWorkerStartedAt $workerID
            $detail = Invoke-StageApi -Method Get -Path "/api/v1/users/me/videos/$VideoID" -Session $Owner.session -AccessToken $Owner.accessToken
            if ($detail.data.status -eq "failed") { throw "transcode failed after Worker restart: $($detail.data.processing_error)" }
            if ($startedAt -gt $startedAtBefore -and $detail.data.status -eq "ready") { break }
        } while ([DateTime]::UtcNow -lt $recoveryDeadline)
        Assert-True ($startedAt -gt $startedAtBefore) "the isolated Worker container was started again after forced exit"
        Assert-Equal $detail.data.status "ready" "the uncommitted event is redelivered and the video reaches ready"
        $job = Get-StageTranscodeJobState $VideoID
        Assert-Equal $job.status 4 "recovered Worker leaves one succeeded transcode job"
        Assert-True ($job.attempts -ge 2) "recovered Worker reclaims the running job after the mid-flight exit"
        Assert-Equal $job.videoCount 1 "Worker recovery keeps exactly one video row"
        Write-Host ("      Worker restarted at {0:o}; job status={1} (succeeded), attempts={2}, videos={3}" -f $startedAt, $job.status, $job.attempts, $job.videoCount)
    } finally {
        Invoke-StageCompose @("start", "worker") | Out-Null
    }
}

function Invoke-StageDuplicateKafkaDrill([uint64]$VideoID) {
    $payloadText = Get-StageOutboxPayload $VideoID
    $payload = $payloadText | ConvertFrom-Json
    if ([string]::IsNullOrWhiteSpace([string]$payload.event_id) -or [string]::IsNullOrWhiteSpace([string]$payload.job_id)) {
        throw "the published synthetic outbox payload is missing stable event/job identifiers"
    }
    $handledBefore = 0
    $logDeadline = [DateTime]::UtcNow.AddSeconds(20)
    do {
        $logs = Invoke-StageCompose @("logs", "--no-color", "--since=5m", "worker")
        $handledBefore = @($logs | Where-Object { $_ -match "transcode event handled" -and $_ -match [regex]::Escape([string]$payload.event_id) }).Count
        if ($handledBefore -ge 1) { break }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $logDeadline)
    Assert-True ($handledBefore -ge 1) "original event has been handled before replay"

    Publish-StageDuplicateKafkaEvent $VideoID $payloadText
    $replayDeadline = [DateTime]::UtcNow.AddSeconds(30)
    $handledAfter = $handledBefore
    $job = $null
    do {
        Start-Sleep -Milliseconds 500
        $logs = Invoke-StageCompose @("logs", "--no-color", "--since=5m", "worker")
        $handledAfter = @($logs | Where-Object { $_ -match "transcode event handled" -and $_ -match [regex]::Escape([string]$payload.event_id) }).Count
        $job = Get-StageTranscodeJobState $VideoID
        if ($handledAfter -gt $handledBefore) { break }
    } while ([DateTime]::UtcNow -lt $replayDeadline)
    Assert-True ($handledAfter -gt $handledBefore) "Worker consumed and acknowledged the duplicate Kafka event"
    Assert-Equal $job.status 4 "duplicate event leaves the original job succeeded"
    Assert-Equal $job.attempts 1 "duplicate event does not re-run a succeeded job"
    Assert-Equal $job.videoCount 1 "duplicate event creates no second video row"
    Write-Host ("      duplicate event_id={0} acknowledged; succeeded job attempts={1}, videos={2}" -f $payload.event_id, $job.attempts, $job.videoCount)
}

function New-StageVideo($Owner, [uint64]$CategoryID, [string[]]$Tags) {
    $fixtureDuration = 8
    $fixtureResolution = "640x360"
    if ($script:DrillWorkerExit) {
        # Keep FFmpeg busy long enough to force-exit the Worker after Claim
        # but before it can finish and commit the Kafka offset.
        $fixtureDuration = 60
        $fixtureResolution = "1280x720"
    }
    Write-Host ("      generate a {0}-second synthetic {1} MP4 fixture" -f $fixtureDuration, $fixtureResolution)
    $ffmpeg = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if (-not $ffmpeg) { throw "ffmpeg is required on PATH to create the synthetic E2E fixture" }
    & $ffmpeg.Source -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=${fixtureResolution}:rate=24" -f lavfi -i "anullsrc=r=44100:cl=stereo" -t $fixtureDuration -shortest -c:v libx264 -preset ultrafast -crf 28 -pix_fmt yuv420p -c:a aac $script:tempVideo
    if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed to generate the synthetic fixture" }
    $file = Get-Item -LiteralPath $script:tempVideo
    $title = "Stage 5 isolated E2E " + [DateTime]::UtcNow.ToString("yyyyMMddHHmmssfff")
    $created = Invoke-StageApi -Method Post -Path "/api/v1/videos" -Session $Owner.session -AccessToken $Owner.accessToken -ExpectStatus @(201) -Body @{
        title = $title
        description = "Synthetic T11 end-to-end fixture; no private media is used."
        file_name = $file.Name
        content_type = "video/mp4"
        file_size = $file.Length
        category_id = $CategoryID
        tags = $Tags
    }
    $id = [uint64]$created.data.id
    $script:videoID = $id
    $script:videoHeaders = $Owner
    Assert-True ($id -gt 0 -and $created.data.upload_url) "video creation returns an ID and presigned upload URL"
    $upload = Invoke-WebRequest -Method Put -Uri $created.data.upload_url -InFile $script:tempVideo -ContentType "video/mp4" -UseBasicParsing -TimeoutSec 90
    if ([int]$upload.StatusCode -ne 200) { throw "MinIO direct upload returned $([int]$upload.StatusCode)" }
    if ($script:DrillKafkaOutage) {
        Write-Host "      stop only the isolated Kafka service, then enqueue the transcode outbox event"
        try {
            Invoke-StageCompose @("stop", "kafka") | Out-Null
            Invoke-StageApi -Method Post -Path "/api/v1/videos/$id/complete" -Session $Owner.session -AccessToken $Owner.accessToken -ExpectStatus @(202) | Out-Null
            $outboxDeadline = [DateTime]::UtcNow.AddSeconds(45)
            $outboxState = $null
            do {
                $outboxState = Get-StageOutboxState $id
                if ($outboxState.attempts -ge 2 -and $outboxState.status -eq 1 -and -not [string]::IsNullOrWhiteSpace($outboxState.lastError)) { break }
                Start-Sleep -Milliseconds 500
            } while ([DateTime]::UtcNow -lt $outboxDeadline)
            Assert-True ($null -ne $outboxState -and $outboxState.attempts -ge 2 -and $outboxState.status -eq 1 -and -not [string]::IsNullOrWhiteSpace($outboxState.lastError)) "Kafka outage records a publish error and a second dispatcher claim while leaving the outbox pending"
            Write-Host ("      observed pending outbox after Kafka outage: status={0}, attempts={1}, last_error_present=True" -f $outboxState.status, $outboxState.attempts)
        } finally {
            Invoke-StageCompose @("start", "kafka") | Out-Null
            Wait-StageKafkaHealthy
        }
    } else {
        Invoke-StageApi -Method Post -Path "/api/v1/videos/$id/complete" -Session $Owner.session -AccessToken $Owner.accessToken -ExpectStatus @(202) | Out-Null
    }

    if ($script:DrillWorkerExit) { Invoke-StageWorkerExitDrill $Owner $id }

    $deadline = [DateTime]::UtcNow.AddSeconds($script:TimeoutSeconds)
    $ownerDetail = $null
    do {
        $ownerDetail = Invoke-StageApi -Method Get -Path "/api/v1/users/me/videos/$id" -Session $Owner.session -AccessToken $Owner.accessToken
        if ($ownerDetail.data.status -eq "failed") { throw "transcode failed: $($ownerDetail.data.processing_error)" }
        if ($ownerDetail.data.status -eq "ready") { break }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($null -eq $ownerDetail -or $ownerDetail.data.status -ne "ready") { throw "video did not become ready within $script:TimeoutSeconds seconds" }
    if ($script:DrillKafkaOutage) {
        $recoveredOutbox = Get-StageOutboxState $id
        Assert-Equal $recoveredOutbox.status 2 "Kafka recovery publishes the pending outbox event"
        Assert-True ($recoveredOutbox.attempts -ge 2) "the event was claimed again after the recorded publish failure"
        Assert-True ([string]::IsNullOrWhiteSpace($recoveredOutbox.lastError)) "successful publish clears the previous outbox error"
        Write-Host ("      recovered outbox: status={0} (published), dispatch_claims={1}, last_error_cleared=True; Worker video is ready" -f $recoveredOutbox.status, $recoveredOutbox.attempts)
    }
    return @{ id = $id; title = $title; owner = $Owner; durationMS = [uint64]$ownerDetail.data.duration_ms }
}

function First-ManifestLine([string]$Manifest) {
    return ($Manifest -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -and -not $_.StartsWith("#") } | Select-Object -First 1)
}

try {
    Write-Host "[1/12] check isolated Compose stack, frontend proxy and API readiness"
    Reset-StageRegistrationBucket
    $health = Invoke-RestMethod -Uri "$BaseUrl/healthz" -TimeoutSec 10
    if ($health.status -ne "ok" -and $health.status -ne "healthy") { throw "frontend health endpoint returned an unexpected response" }
    $readyDeadline = [DateTime]::UtcNow.AddSeconds(60)
    $ready = $null
    do {
        try {
            $ready = Invoke-StageApi -Method Get -Path "/api/v1/categories" -TimeoutSec 5
            if (@($ready.data.items).Count -gt 0) { break }
        } catch { Start-Sleep -Seconds 2 }
    } while ([DateTime]::UtcNow -lt $readyDeadline)
    if ($null -eq $ready -or @($ready.data.items).Count -eq 0) { throw "real API did not become ready through the same-origin proxy" }
    if ($DrillRedisOutage) { Invoke-StageRedisOutageDrill }

    Write-Host "[2/12] create isolated author, viewer and admin identities; exercise CSRF login and refresh rotation"
    $author = Connect-StageIdentity (New-StageIdentity "stage5author" "Stage 5 Author")
    $viewer = Connect-StageIdentity (New-StageIdentity "stage5viewer" "Stage 5 Viewer")
    $admin = New-StageIdentity "stage5admin" "Stage 5 Admin"
    if ($author.id -eq $viewer.id -or $author.id -eq $admin.id -or $viewer.id -eq $admin.id) { throw "isolated E2E identities are not unique" }
    $grant = Invoke-StageCompose @("exec", "-T", "api", "/app/api", "admin", "grant", $admin.username)
    $admin = Connect-StageIdentity $admin
    Assert-Equal $admin.user.role "admin" "explicit CLI grant is reflected after a fresh login"
    $oldAccess = $author.accessToken
    $oldRefresh = $author.session.Cookies.GetCookies([Uri]("$BaseUrl/api/v1/auth/refresh")) | Where-Object { $_.Name -eq "video_share_refresh" } | Select-Object -First 1
    Assert-True ($null -ne $oldRefresh -and $oldRefresh.HttpOnly) "refresh cookie is HttpOnly"
    $refresh = Invoke-StageApi -Method Post -Path "/api/v1/auth/refresh" -Session $author.session -Body @{}
    Assert-True ($refresh.data.access_token -and $refresh.data.access_token -ne $oldAccess) "refresh rotates the access token"
    $author.accessToken = [string]$refresh.data.access_token
    Invoke-StageApi -Method Get -Path "/api/v1/users/me" -Session $author.session -AccessToken $author.accessToken | Out-Null

    Write-Host "[3/12] update profile and select an enabled MySQL-backed category"
    $profile = Invoke-StageApi -Method Patch -Path "/api/v1/users/me" -Session $author.session -AccessToken $author.accessToken -Body @{ nickname = "Stage 5 Public Author"; bio = "Synthetic acceptance account" }
    Assert-Equal $profile.data.nickname "Stage 5 Public Author" "profile mutation is persisted"
    $categories = Invoke-StageApi -Method Get -Path "/api/v1/categories"
    $category = @($categories.data.items | Where-Object { $_.enabled -eq $true }) | Select-Object -First 1
    if ($null -eq $category) { throw "no enabled category is available" }

    Write-Host "[4/12] upload synthetic video, wait for real Worker transcode, and verify HLS assets"
    $video = New-StageVideo $author ([uint64]$category.id) @("stage5", "acceptance")
    $detail = Invoke-StageApi -Method Get -Path "/api/v1/videos/$($video.id)"
    Assert-Equal $detail.data.play_type "hls" "ready video uses HLS playback"
    $master = Invoke-StageApi -Method Get -Path $detail.data.play_url
    Assert-True ($master.raw -match "#EXTM3U") "master playlist is returned by the real API/MinIO path"
    $variantPath = First-ManifestLine $master.raw
    if (-not $variantPath) { throw "HLS master playlist has no variant" }
    $variant = Invoke-StageApi -Method Get -Path $variantPath
    $segmentPath = First-ManifestLine $variant.raw
    if (-not $segmentPath) { throw "HLS variant playlist has no media segment" }
    $segment = Invoke-WebRequest -Uri (Resolve-StageUrl $segmentPath) -UseBasicParsing -TimeoutSec 30
    if ([int]$segment.StatusCode -ne 200 -or $segment.RawContentLength -le 0) { throw "HLS segment is empty or unavailable" }
    $cover = Invoke-WebRequest -Uri (Resolve-StageUrl $detail.data.cover_url) -UseBasicParsing -TimeoutSec 30
    if ([int]$cover.StatusCode -ne 200 -or $cover.RawContentLength -le 0) { throw "transcoded cover is empty or unavailable" }
    if ($DrillDuplicateKafkaEvent) { Invoke-StageDuplicateKafkaDrill $video.id }

    Write-Host "[5/12] discover, share the creator page, follow, like, favorite and create an idempotent comment"
    $query = [Uri]::EscapeDataString($video.title)
    $discover = Invoke-StageApi -Method Get -Path "/api/v1/videos?q=$query&page_size=50"
    Assert-True (@($discover.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "new public video is discoverable"
    $creator = Invoke-StageApi -Method Get -Path "/api/v1/users/$($author.id)"
    Assert-Equal ([uint64]$creator.data.id) $author.id "public creator page resolves without login"
    $creatorVideos = Invoke-StageApi -Method Get -Path "/api/v1/users/$($author.id)/videos?page=1&page_size=20"
    Assert-True (@($creatorVideos.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "creator page lists the public video"
    $follow = Invoke-StageApi -Method Put -Path "/api/v1/users/$($author.id)/follow" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{}
    Assert-Equal $follow.data.following $true "viewer follows author"
    $like = Invoke-StageApi -Method Put -Path "/api/v1/videos/$($video.id)/like" -Session $viewer.session -AccessToken $viewer.accessToken
    Assert-Equal $like.data.active $true "viewer likes video"
    $favorite = Invoke-StageApi -Method Put -Path "/api/v1/videos/$($video.id)/favorite" -Session $viewer.session -AccessToken $viewer.accessToken
    Assert-Equal $favorite.data.active $true "viewer favorites video"
    $rootRequestID = "stage5-root-" + [guid]::NewGuid().ToString("N")
    $rootBody = @{ content = "Synthetic T11 root comment"; request_id = $rootRequestID }
    $root = Invoke-StageApi -Method Post -Path "/api/v1/videos/$($video.id)/comments" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(201) -Body $rootBody
    $rootReplay = Invoke-StageApi -Method Post -Path "/api/v1/videos/$($video.id)/comments" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(201) -Body $rootBody
    Assert-Equal ([uint64]$rootReplay.data.id) ([uint64]$root.data.id) "replayed request_id returns the same comment"
    $reply = Invoke-StageApi -Method Post -Path "/api/v1/videos/$($video.id)/comments" -Session $author.session -AccessToken $author.accessToken -ExpectStatus @(201) -Body @{
        content = "Synthetic T11 reply"
        parent_id = [uint64]$root.data.id
        request_id = "stage5-reply-" + [guid]::NewGuid().ToString("N")
    }
    $replies = Invoke-StageApi -Method Get -Path "/api/v1/comments/$($root.data.id)/replies?page=1&page_size=20"
    Assert-True (@($replies.data.items | Where-Object { [uint64]$_.id -eq [uint64]$reply.data.id }).Count -eq 1) "one-level reply is returned under its root"

    Write-Host "[6/12] verify comment/reply notifications and mark a notification read"
    $authorNotifications = Invoke-StageApi -Method Get -Path "/api/v1/notifications?page=1&page_size=50" -Session $author.session -AccessToken $author.accessToken
    Assert-True (@($authorNotifications.data.items | Where-Object { [uint64]$_.video_id -eq $video.id }).Count -ge 1) "video author receives comment or follow notification"
    $viewerNotifications = Invoke-StageApi -Method Get -Path "/api/v1/notifications?page=1&page_size=50" -Session $viewer.session -AccessToken $viewer.accessToken
    $replyNotice = @($viewerNotifications.data.items | Where-Object { [uint64]$_.comment_id -eq [uint64]$reply.data.id }) | Select-Object -First 1
    Assert-True ($null -ne $replyNotice) "root commenter receives a notification for the reply"
    Invoke-StageApi -Method Patch -Path "/api/v1/notifications/$($replyNotice.id)/read" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{} | Out-Null

    Write-Host "[7/12] verify watch heartbeat idempotency, resume position and creator analytics"
    $watch1 = Invoke-StageApi -Method Post -Path "/api/v1/videos/$($video.id)/watch-sessions" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(201) -Body @{}
    Start-Sleep -Milliseconds 1300
    $watch1Beat = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch1.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = 1; position_ms = 3000; watched_delta_ms = 3000; ended = $false }
    $watch1Duplicate = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch1.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = 1; position_ms = 3000; watched_delta_ms = 3000; ended = $false }
    Assert-Equal ([uint64]$watch1Duplicate.data.accepted_delta_ms) 0 "duplicate heartbeat sequence grants no extra watch time"
    $watch2 = Invoke-StageApi -Method Post -Path "/api/v1/videos/$($video.id)/watch-sessions" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(201) -Body @{}
    Assert-Equal ([uint64]$watch2.data.resume_position_ms) 3000 "new playback session resumes from saved progress"
    Start-Sleep -Milliseconds 3300
    $watch2First = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch2.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = 1; position_ms = 6000; watched_delta_ms = 15000; ended = $false }
    Assert-True $watch2First.data.qualified "wall-clock bounded watch time qualifies the playback session"
    $watch2Replay = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch2.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = 1; position_ms = 6000; watched_delta_ms = 15000; ended = $false }
    Assert-Equal ([uint64]$watch2Replay.data.accepted_delta_ms) 0 "replayed qualified heartbeat remains idempotent"
    $finalSeq = 2
    if ([uint64]$video.durationMS -gt 30000) {
        $position = 6000
        for ($seq = 2; $seq -le 6; $seq++) {
            Start-Sleep -Seconds 10
            $position = [Math]::Min([int]$video.durationMS - 3000, $position + 10000)
            $credit = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch2.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = $seq; position_ms = $position; watched_delta_ms = 10000; ended = $false }
            Assert-True ([uint64]$credit.data.accepted_delta_ms -gt 0) "long synthetic video receives only elapsed wall-clock watch credit"
        }
        $finalSeq = 7
    } else {
        Start-Sleep -Milliseconds 3300
    }
    $finalPosition = [Math]::Max(0, [int]$video.durationMS - 500)
    $watch2End = Invoke-StageApi -Method Post -Path "/api/v1/watch-sessions/$($watch2.data.session_id)/heartbeat" -Session $viewer.session -AccessToken $viewer.accessToken -Body @{ seq = $finalSeq; position_ms = $finalPosition; watched_delta_ms = 15000; ended = $true }
    Assert-True $watch2End.data.completed "completion requires both near-end position and sufficient wall-clock watch time"
    $summary = Invoke-StageApi -Method Get -Path "/api/v1/users/me/creator/summary?days=7" -Session $author.session -AccessToken $author.accessToken
    Assert-True ([int64]$summary.data.current.view_count -ge 1) "creator analytics includes real qualified viewing"

    Write-Host "[8/12] rebuild and read the Redis day ranking from this synthetic watch metric"
    Invoke-StageCompose @("exec", "-T", "-e", "RANKING_REBUILD_ONCE=1", "ranking-rebuild", "/app/ranking-rebuild", "day") | Out-Null
    $ranking = Invoke-StageApi -Method Get -Path "/api/v1/videos/ranking?window=day&page=1&page_size=50"
    Assert-True (@($ranking.data.items | Where-Object { [uint64]$_.video_id -eq $video.id -or [uint64]$_.id -eq $video.id }).Count -ge 1) "qualified public video appears in the rebuilt day ranking"
    if ($DrillCacheExpiry) { Invoke-StageCacheExpiryDrill }

    Write-Host "[9/12] verify report privacy, admin assignment, resolution, hide and restore"
    Invoke-StageApi -Method Get -Path "/api/v1/admin/reports" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(403) | Out-Null
    $report = Invoke-StageApi -Method Post -Path "/api/v1/reports" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(201) -Body @{
        target_type = "video"; target_id = $video.id; reason_code = "other"; detail = "Synthetic isolated T11 moderation drill"; request_id = "stage5-report-" + [guid]::NewGuid().ToString("N")
    }
    $mineReports = Invoke-StageApi -Method Get -Path "/api/v1/users/me/reports?page=1&page_size=20" -Session $viewer.session -AccessToken $viewer.accessToken
    Assert-True (@($mineReports.data.items | Where-Object { [uint64]$_.id -eq [uint64]$report.data.id }).Count -eq 1) "reporter can read own report"
    $assigned = Invoke-StageApi -Method Patch -Path "/api/v1/admin/reports/$($report.data.id)/assign" -Session $admin.session -AccessToken $admin.accessToken -RequestID ("stage5-assign-" + [guid]::NewGuid().ToString("N")) -Body @{}
    Assert-Equal $assigned.data.status "investigating" "admin assignment moves report to investigating"
    $resolved = Invoke-StageApi -Method Patch -Path "/api/v1/admin/reports/$($report.data.id)" -Session $admin.session -AccessToken $admin.accessToken -Body @{
        status = "resolved"; resolution_reason = "Isolated E2E moderation resolution"; action = "hide_video"; reason = "Synthetic content lifecycle"; request_id = "stage5-resolve-" + [guid]::NewGuid().ToString("N")
    }
    Assert-Equal $resolved.data.status "resolved" "admin resolves the report transactionally"
    Invoke-StageApi -Method Get -Path "/api/v1/videos/$($video.id)" -ExpectStatus @(404) | Out-Null
    $restore = Invoke-StageApi -Method Post -Path "/api/v1/admin/videos/$($video.id)/restore_video" -Session $admin.session -AccessToken $admin.accessToken -Body @{
        reason = "Synthetic recovery after moderation test"; request_id = "stage5-restore-" + [guid]::NewGuid().ToString("N"); report_id = [uint64]$report.data.id
    }
    Assert-Equal $restore.data.after_state "visible" "admin restore creates an auditable recovery transition"
    Invoke-StageApi -Method Get -Path "/api/v1/videos/$($video.id)" | Out-Null
    $audit = Invoke-StageApi -Method Get -Path "/api/v1/admin/moderation-actions?page=1&page_size=50" -Session $admin.session -AccessToken $admin.accessToken
    Assert-True (@($audit.data.items | Where-Object { [uint64]$_.target_id -eq $video.id }).Count -ge 2) "hide and restore are both represented in the audit log"

    Write-Host "[10/12] run actual-browser smoke against the deployed Nginx origin"
    if (-not $SkipBrowser) {
        $env:STAGE5_E2E_BASE_URL = $BaseUrl
        $env:STAGE5_E2E_USERNAME = $viewer.username
        $env:STAGE5_E2E_PASSWORD = $viewer.password
        $env:STAGE5_E2E_VIDEO_ID = [string]$video.id
        $env:STAGE5_E2E_VIDEO_TITLE = $video.title
        Push-Location (Join-Path $repoRoot "frontend")
        try {
            & npm.cmd run test:e2e:stage5
            if ($LASTEXITCODE -ne 0) { throw "real-browser Stage 5 Playwright smoke failed with exit code $LASTEXITCODE" }
        } finally {
            Pop-Location
        }
    } else {
        Write-Warning "browser E2E skipped by explicit -SkipBrowser; this is not a T11 acceptance pass"
    }

    Write-Host "[11/12] logout revokes the session family and rejects the prior access token"
    $viewerOldToken = $viewer.accessToken
    Invoke-StageApi -Method Post -Path "/api/v1/auth/logout" -Session $viewer.session -AccessToken $viewer.accessToken -ExpectStatus @(204) -Body @{} | Out-Null
    Invoke-StageApi -Method Get -Path "/api/v1/users/me" -Session $viewer.session -AccessToken $viewerOldToken -ExpectStatus @(401) | Out-Null

    Write-Host "[12/12] soft-delete only the synthetic E2E video and report the created identities"
    Invoke-StageApi -Method Delete -Path "/api/v1/users/me/videos/$($video.id)" -Session $author.session -AccessToken $author.accessToken | Out-Null
    $videoID = [uint64]0
    $videoHeaders = $null
    if ($SkipBrowser) {
        Write-Host "Stage 5 API E2E passed; browser E2E was explicitly skipped."
    } else {
        Write-Host "Stage 5 API + browser E2E passed against the isolated Compose stack."
    }
    Write-Host ("      synthetic IDs: author={0}, viewer={1}, admin={2}, video={3}, report={4}" -f $author.id, $viewer.id, $admin.id, $video.id, $report.data.id)
    Write-Host "      generated passwords were not printed; synthetic identities and audit history remain only in the isolated smoke database."
} finally {
    if ($videoID -ne 0 -and $null -ne $videoHeaders) {
        try {
            Invoke-StageApi -Method Delete -Path "/api/v1/users/me/videos/$videoID" -Session $videoHeaders.session -AccessToken $videoHeaders.accessToken -ExpectStatus @(200, 404) | Out-Null
            Write-Host "      soft-deleted only the synthetic video video_id=$videoID"
        } catch { Write-Warning ("could not soft-delete synthetic video {0}: {1}" -f $videoID, $_.Exception.Message) }
    }
    Remove-Item -LiteralPath $tempVideo -Force -ErrorAction SilentlyContinue
    foreach ($name in $browserEnvNames) { [Environment]::SetEnvironmentVariable($name, $priorBrowserEnv[$name], "Process") }
}
