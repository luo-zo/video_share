param(
    [ValidateSet("smoke", "baseline", "stress", "media")]
    [string]$Mode = "baseline",
    [string]$ComposeProject = "stage5-smoke-20260924",
    [string]$BaseUrl = "http://127.0.0.1:18081",
    [int]$VUs = 5,
    [string]$Duration = "15s",
    [int]$Repeat = 3
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$BaseUrl = $BaseUrl.TrimEnd("/")
$baseUri = [Uri]$BaseUrl
if ($baseUri.Scheme -ne "http" -or $baseUri.Host -notin @("127.0.0.1", "localhost", "::1") -or $baseUri.Port -ne 18081) {
    throw "Stage 5 benchmark only permits the isolated local API at http://127.0.0.1:18081."
}
if ($ComposeProject -notmatch "^stage5-smoke-[a-zA-Z0-9_-]+$") {
    throw "ComposeProject must be an explicitly isolated stage5-smoke-* project."
}
if ($VUs -lt 1 -or $VUs -gt 10) { throw "VUs must be between 1 and 10 for this local benchmark." }
if ($Repeat -lt 3 -or $Repeat -gt 5) { throw "Repeat must be between 3 and 5 to preserve the comparison contract." }
if ($Duration -notmatch "^([1-9][0-9]*)(s|m)$" -or ([int]$Matches[1] -gt $(if ($Matches[2] -eq "s") { 60 } else { 1 }))) {
    throw "Duration must be explicit and no longer than 60 seconds (for example, 15s or 1m)."
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$composeFiles = @(
    (Join-Path $repoRoot "backend/deploy/docker-compose.yml"),
    (Join-Path $repoRoot "backend/deploy/docker-compose.isolated-smoke.yml")
)
$composeBase = @("compose", "-p", $ComposeProject, "-f", $composeFiles[0], "-f", $composeFiles[1])
$scriptDir = Join-Path $repoRoot "perf/k6"
$resultsDir = Join-Path $repoRoot "docs/stage5/perf-results"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$k6Image = "grafana/k6:2.3.0"
$csvPath = Join-Path $resultsDir "summary-$timestamp.csv"
$resultRows = New-Object System.Collections.Generic.List[object]
$redisWasStoppedByScript = $false

New-Item -ItemType Directory -Force -Path $resultsDir | Out-Null

function Invoke-Compose([string[]]$Arguments) {
    $all = @($script:composeBase) + $Arguments
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($exitCode -ne 0) {
        throw ("docker {0} failed: {1}" -f ($Arguments -join " "), ($output -join "`n"))
    }
    return ,@($output)
}

function Invoke-ComposeOptional([string[]]$Arguments) {
    $all = @($script:composeBase) + $Arguments
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($exitCode -ne 0) { return $null }
    return ,@($output)
}

function Invoke-RedisInfo {
    $command = 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli --no-auth-warning INFO stats'
    $lines = Invoke-Compose @("exec", "-T", "redis", "sh", "-c", $command)
    $values = @{}
    foreach ($line in $lines) {
        if ([string]$line -match "^(keyspace_hits|keyspace_misses):([0-9]+)") {
            $values[$Matches[1]] = [int64]$Matches[2]
        }
    }
    return $values
}

function Invoke-RedisDelete([string[]]$Keys) {
    foreach ($key in $Keys) {
        if ($key -notmatch "^video_share:(cache:categories:v1|rank:(day|week):\d{4}-\d{2}-\d{2}(:generated_at)?)$") {
            throw "Refusing to delete an unrecognized cache key."
        }
    }
    $command = 'REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli --no-auth-warning DEL "$@"'
    $args = @("exec", "-T", "redis", "sh", "-c", $command, "stage5-bench-key-delete") + $Keys
    $output = Invoke-Compose $args
    if (($output -join "`n") -notmatch "(?m)^\d+$") { throw "Redis did not confirm the exact-key cache deletion." }
}

function Get-MySQLQuestions {
    $command = 'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" -D "$MYSQL_DATABASE" -e ''SHOW GLOBAL STATUS'''
    $all = @($script:composeBase) + @("exec", "-T", "mysql", "sh", "-c", $command)
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $output = & docker @all 2>&1
        $exitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($exitCode -ne 0) { return $null }
    $text = (@($output) | ForEach-Object { [string]$_ }) -join "`n"
    $match = [regex]::Match($text, "(?m)^Questions\s+([0-9]+)")
    if ($match.Success) { return [int64]$match.Groups[1].Value }
    return $null
}

function Convert-MemoryToMiB([string]$value) {
    $match = [regex]::Match($value, "^\s*([0-9]+(?:\.[0-9]+)?)\s*(B|kB|MB|GB|KiB|MiB|GiB|TiB)")
    if (-not $match.Success) { return $null }
    $number = [double]::Parse($match.Groups[1].Value, [Globalization.CultureInfo]::InvariantCulture)
    switch ($match.Groups[2].Value) {
        "B" { return $number / 1MB }
        "kB" { return $number / 1KB }
        "MB" { return $number }
        "GB" { return $number * 1024 }
        "KiB" { return $number / 1024 }
        "MiB" { return $number }
        "GiB" { return $number * 1024 }
        "TiB" { return $number * 1024 * 1024 }
    }
}

function Measure-DockerStats([string]$StatsPath) {
    $samples = @{}
    if (-not (Test-Path -LiteralPath $StatsPath)) { return @{ cpuAverage = $null; cpuPeak = $null; memoryPeaks = @{} } }
    foreach ($line in Get-Content -LiteralPath $StatsPath) {
        $parts = ([string]$line).Trim() -split "\|", 3
        if ($parts.Count -ne 3 -or $parts[1] -notmatch "([0-9]+(?:\.[0-9]+)?)%" ) { continue }
        $cpuText = $Matches[1]
        $name = $parts[0]
        $service = if ($name -match "-(api|mysql|redis|minio)-[0-9]+$") { $Matches[1] } else { $name }
        $cpu = [double]::Parse($cpuText, [Globalization.CultureInfo]::InvariantCulture)
        $memory = Convert-MemoryToMiB (($parts[2] -split "/", 2)[0])
        if (-not $samples.ContainsKey($service)) { $samples[$service] = @{ cpu = @(); memory = @() } }
        $samples[$service].cpu += $cpu
        if ($null -ne $memory) { $samples[$service].memory += $memory }
    }
    $allCPU = @($samples.Values | ForEach-Object { $_.cpu } | ForEach-Object { $_ })
    $cpuAverage = $null
    $cpuPeak = $null
    if ($allCPU.Count -gt 0) {
        $cpuAverage = [Math]::Round(($allCPU | Measure-Object -Average).Average, 3)
        $cpuPeak = [Math]::Round(($allCPU | Measure-Object -Maximum).Maximum, 3)
    }
    $memoryPeaks = @{}
    foreach ($service in $samples.Keys) {
        if ($samples[$service].memory.Count -gt 0) {
            $memoryPeaks[$service] = [Math]::Round(($samples[$service].memory | Measure-Object -Maximum).Maximum, 3)
        }
    }
    return @{ cpuAverage = $cpuAverage; cpuPeak = $cpuPeak; memoryPeaks = $memoryPeaks }
}

function Get-CurrentRedisKeys([string]$Date) {
    return @(
        "video_share:rank:day:$Date",
        "video_share:rank:day:$Date`:generated_at"
    )
}

function Invoke-K6([string]$ScriptName, [string]$Profile, [string]$CacheMode, [int]$RunNumber, [string]$VideoID = "", [string]$SegmentPath = "") {
    $name = [IO.Path]::GetFileNameWithoutExtension($ScriptName)
    $stem = "{0}-{1}-{2}-r{3}-{4}" -f $timestamp, $name, $CacheMode, $RunNumber, $Profile
    $summaryName = "$stem.json"
    $summaryPath = Join-Path $resultsDir $summaryName
    $statsPath = Join-Path $resultsDir "$stem.stats.txt"
    $statsError = Join-Path $resultsDir "$stem.stats.err.txt"
    $scriptMount = ($scriptDir -replace "\\", "/") + ":/scripts:ro"
    $resultsMount = ($resultsDir -replace "\\", "/") + ":/results"
    $envArgs = @(
        "-e", $(if ($ScriptName -eq "hls-segment.js") { "BASE_URL=http://127.0.0.1:18081" } else { "BASE_URL=http://host.docker.internal:18081" }),
        "-e", "PROFILE=$Profile",
        "-e", "CACHE_MODE=$CacheMode",
        "-e", "VUS=$VUs",
        "-e", "DURATION=$Duration"
    )
    if ($ScriptName -eq "ranking.js") { $envArgs += @("-e", "WINDOW=day") }
    if ($VideoID) { $envArgs += @("-e", "VIDEO_ID=$VideoID") }
    if ($SegmentPath) { $envArgs += @("-e", "SEGMENT_PATH=$SegmentPath") }
    $services = @("api", "mysql")
    if ($ScriptName -eq "hls-segment.js") { $services += "minio" }
    if ($CacheMode -ne "redis-unavailable" -and $ScriptName -notin @("engagement.js", "hls-segment.js")) { $services += "redis" }
    $idsOutput = Invoke-Compose (@("ps", "-q") + $services)
    $containerIDs = @($idsOutput | ForEach-Object { ([string]$_).Trim() } | Where-Object { $_ -match "^[a-f0-9]{12,64}$" })
    $statsProcess = $null
    if ($containerIDs.Count -gt 0) {
        $statsArguments = @("stats", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}") + $containerIDs
        $statsProcess = Start-Process -FilePath "docker" -ArgumentList $statsArguments -WindowStyle Hidden -RedirectStandardOutput $statsPath -RedirectStandardError $statsError -PassThru
        Start-Sleep -Milliseconds 350
    }
    $redisBefore = $null
    if ($ScriptName -notin @("engagement.js", "hls-segment.js")) { try { $redisBefore = Invoke-RedisInfo } catch { } }
    $sqlBefore = Get-MySQLQuestions
    try {
        $dockerRunOptions = @("run", "--rm")
        if ($ScriptName -eq "hls-segment.js") {
            # Local MinIO presigned redirects are signed for 127.0.0.1:19000;
            # host networking lets this read-only k6 container reach that exact host.
            $dockerRunOptions += @("--network", "host")
        }
        $dockerArgs = $dockerRunOptions + @(
            "-v", $scriptMount,
            "-v", $resultsMount
        ) + $envArgs + @(
            $k6Image,
            "run",
            "--summary-export=/results/$summaryName",
            "/scripts/$ScriptName"
        )
        & docker @dockerArgs
        $k6Exit = $LASTEXITCODE
        if ($k6Exit -ne 0) { throw "k6 $ScriptName $CacheMode run $RunNumber failed with exit code $k6Exit." }
    } finally {
        if ($null -ne $statsProcess -and -not $statsProcess.HasExited) {
            Stop-Process -Id $statsProcess.Id -Force
            try { $statsProcess.WaitForExit() } catch { }
        }
    }
    $redisAfter = $null
    if ($ScriptName -notin @("engagement.js", "hls-segment.js")) { try { $redisAfter = Invoke-RedisInfo } catch { } }
    $sqlAfter = Get-MySQLQuestions
    $stats = Measure-DockerStats $statsPath
    $redisHitDelta = $null
    $redisMissDelta = $null
    if ($null -ne $redisBefore -and $null -ne $redisAfter) {
        $redisHitDelta = [int64]$redisAfter.keyspace_hits - [int64]$redisBefore.keyspace_hits
        $redisMissDelta = [int64]$redisAfter.keyspace_misses - [int64]$redisBefore.keyspace_misses
    }
    $sqlDelta = $null
    if ($null -ne $sqlBefore -and $null -ne $sqlAfter) { $sqlDelta = [int64]$sqlAfter - [int64]$sqlBefore }
    $resultRows.Add([pscustomobject]@{
        timestamp = (Get-Date).ToString("o")
        endpoint = $name
        cache_mode = $CacheMode
        profile = $Profile
        repeat = $RunNumber
        vus = $(if ($Profile -eq "cold") { 1 } else { $VUs })
        duration = $(if ($Profile -eq "cold") { "1 iteration" } else { $Duration })
        redis_hits_delta = $redisHitDelta
        redis_misses_delta = $redisMissDelta
        mysql_questions_delta = $sqlDelta
        docker_cpu_avg_pct = $stats.cpuAverage
        docker_cpu_peak_pct = $stats.cpuPeak
        api_mem_peak_mib = $stats.memoryPeaks.api
        mysql_mem_peak_mib = $stats.memoryPeaks.mysql
        redis_mem_peak_mib = $stats.memoryPeaks.redis
        minio_mem_peak_mib = $stats.memoryPeaks.minio
        video_id = $VideoID
        segment_path = $SegmentPath
        k6_summary = $summaryName
        docker_stats = [IO.Path]::GetFileName($statsPath)
    }) | Out-Null
    $resultRows | Export-Csv -LiteralPath $csvPath -NoTypeInformation -Encoding UTF8
    Write-Host ("      recorded {0}/{1}/{2} r{3}; MySQL Questions delta={4}, Redis hit delta={5}, miss delta={6}" -f $name, $CacheMode, $Profile, $RunNumber, $sqlDelta, $redisHitDelta, $redisMissDelta)
}

function Warm-CategoryCache {
    $null = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/categories" -TimeoutSec 10
}

function Warm-RankingCache {
    $rebuild = Invoke-Compose @("exec", "-T", "-e", "RANKING_REBUILD_ONCE=1", "ranking-rebuild", "/app/ranking-rebuild", "day")
    $null = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/videos/ranking?window=day&page=1&page_size=20" -TimeoutSec 15
}

function Wait-RedisHealthy {
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        $lines = Invoke-ComposeOptional @("ps", "redis")
        if ($null -ne $lines -and ($lines -join "`n") -match "healthy") { return }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "The isolated Redis service did not become healthy after the benchmark outage phase."
}

function Invoke-ColdRun([string]$ScriptName, [int]$RunNumber) {
    $date = (Get-Date).ToString("yyyy-MM-dd")
    if ($ScriptName -eq "categories.js") {
        Invoke-RedisDelete @("video_share:cache:categories:v1")
    } else {
        Invoke-RedisDelete (Get-CurrentRedisKeys $date)
    }
    Invoke-K6 $ScriptName "cold" "cold" $RunNumber
}

function Invoke-HotRun([string]$ScriptName, [int]$RunNumber, [string]$VideoID = "") {
    if ($ScriptName -eq "categories.js") { Warm-CategoryCache }
    elseif ($ScriptName -eq "ranking.js") { Warm-RankingCache }
    Invoke-K6 $ScriptName "load" "hot" $RunNumber $VideoID
}

function Invoke-UnavailableRun([string]$ScriptName, [int]$RunNumber) {
    Invoke-K6 $ScriptName "load" "redis-unavailable" $RunNumber
}

function Get-IsolatedHLSSegment {
    $ranking = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/videos/ranking?window=day&page=1&page_size=1" -TimeoutSec 15
    $items = @($ranking.data.items)
    if ($items.Count -eq 0) { throw "No public ready video exists in the isolated database for the HLS segment benchmark." }
    $videoID = [string]$items[0].id
    $master = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/api/v1/videos/$videoID/hls/master.m3u8" -TimeoutSec 15
    $masterText = if ($master.Content -is [byte[]]) { [Text.Encoding]::UTF8.GetString($master.Content) } else { [string]$master.Content }
    $childMatch = [regex]::Match($masterText, "(?m)^\s*(/api/v1/videos/$videoID/hls/[A-Za-z0-9._/-]+\.m3u8)\s*$")
    if (-not $childMatch.Success) { throw "The public video master manifest did not contain a safe same-video HLS playlist path." }
    $child = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$($childMatch.Groups[1].Value)" -TimeoutSec 15
    $childText = if ($child.Content -is [byte[]]) { [Text.Encoding]::UTF8.GetString($child.Content) } else { [string]$child.Content }
    $segments = @([regex]::Matches($childText, "(?m)^\s*(/api/v1/videos/$videoID/hls/[A-Za-z0-9._/-]+\.ts)\s*$"))
    if ($segments.Count -eq 0) { throw "The HLS media playlist did not contain a safe same-video MPEG-TS segment path." }
    $segmentPath = $segments[$segments.Count - 1].Groups[1].Value
    if ($segmentPath.Contains("..")) { throw "The HLS media playlist returned a path traversal candidate." }
    $probe = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl$segmentPath" -TimeoutSec 20
    if ([int]$probe.StatusCode -ne 200 -or $probe.Headers["Content-Type"] -notlike "*video/mp2t*") {
        throw "The isolated HLS segment preflight did not return HTTP 200 video/mp2t."
    }
    $bytes = [int]$probe.RawContentLength
    if ($bytes -lt 1024) { throw "The selected HLS segment was unexpectedly small ($bytes bytes)." }
    return [pscustomobject]@{ videoID = $videoID; segmentPath = $segmentPath; bytes = $bytes }
}

try {
    $health = Invoke-RestMethod -Method Get -Uri "$BaseUrl/readyz" -TimeoutSec 5
    if ($health.status -ne "ready") { throw "The isolated API readiness endpoint did not return ready." }
    $composeStatus = Invoke-Compose @("ps", "api", "mysql", "redis", "ranking-rebuild")
    if (($composeStatus -join "`n") -notmatch "Up") { throw "The isolated API/MySQL/Redis/ranking services must already be running; this runner never starts or recreates them." }
    $image = & docker image inspect $k6Image 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Pinned k6 image is absent locally; pulling only $k6Image."
        & docker pull $k6Image
        if ($LASTEXITCODE -ne 0) { throw "Could not pull the pinned k6 image $k6Image." }
    }
    $date = (Get-Date).ToString("yyyy-MM-dd")
    if ($Mode -eq "smoke") {
        Invoke-ColdRun "categories.js" 1
        Invoke-ColdRun "ranking.js" 1
        $ranking = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/videos/ranking?window=day&page=1&page_size=1" -TimeoutSec 15
        if (@($ranking.data.items).Count -eq 0) { throw "No public video exists in the isolated database for the engagement smoke test." }
        Invoke-K6 "engagement.js" "cold" "mysql-read" 1 ([string]$ranking.data.items[0].id)
    } elseif ($Mode -eq "media") {
        if ($VUs -gt 5) { throw "Media mode is intentionally capped at 5 VUs to bound local object-store transfer volume." }
        $segment = Get-IsolatedHLSSegment
        Write-Host ("      HLS fixture video={0}, segment-bytes={1}; running only this isolated media read" -f $segment.videoID, $segment.bytes)
        for ($run = 1; $run -le $Repeat; $run++) {
            Invoke-K6 "hls-segment.js" "load" "object-read" $run $segment.videoID $segment.segmentPath
        }
    } elseif ($Mode -eq "stress") {
        if ($VUs -le 5) { throw "Stress mode must exceed the 5-VU baseline; use -VUs 10." }
        foreach ($scriptName in @("categories.js", "ranking.js")) {
            for ($run = 1; $run -le $Repeat; $run++) { Invoke-HotRun $scriptName $run }
        }
        $ranking = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/videos/ranking?window=day&page=1&page_size=1" -TimeoutSec 15
        if (@($ranking.data.items).Count -eq 0) { throw "No public video exists in the isolated database for the engagement stress test." }
        for ($run = 1; $run -le $Repeat; $run++) { Invoke-K6 "engagement.js" "load" "mysql-read" $run ([string]$ranking.data.items[0].id) }
    } else {
        foreach ($scriptName in @("categories.js", "ranking.js")) {
            for ($run = 1; $run -le $Repeat; $run++) { Invoke-ColdRun $scriptName $run }
        }
        foreach ($scriptName in @("categories.js", "ranking.js")) {
            for ($run = 1; $run -le $Repeat; $run++) { Invoke-HotRun $scriptName $run }
        }

        Write-Host "      stopping only isolated Redis to measure documented MySQL fallback"
        Invoke-Compose @("stop", "redis") | Out-Null
        $redisWasStoppedByScript = $true
        try {
            foreach ($scriptName in @("categories.js", "ranking.js")) {
                for ($run = 1; $run -le $Repeat; $run++) { Invoke-UnavailableRun $scriptName $run }
            }
        } finally {
            Invoke-Compose @("start", "redis") | Out-Null
            $redisWasStoppedByScript = $false
            Wait-RedisHealthy
        }

        $ranking = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/videos/ranking?window=day&page=1&page_size=1" -TimeoutSec 15
        if (@($ranking.data.items).Count -eq 0) { throw "No public video exists in the isolated database for the engagement benchmark." }
        for ($run = 1; $run -le $Repeat; $run++) { Invoke-K6 "engagement.js" "load" "mysql-read" $run ([string]$ranking.data.items[0].id) }
    }
    Write-Host "Benchmark result summary: $csvPath"
    Write-Host "Raw k6 summaries and sampled Docker stats: $resultsDir"
} finally {
    if ($redisWasStoppedByScript) {
        Invoke-Compose @("start", "redis") | Out-Null
        Wait-RedisHealthy
    }
}
