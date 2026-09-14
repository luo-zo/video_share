param(
    [string]$BaseUrl = "http://127.0.0.1:8081",
    [int]$TimeoutSeconds = 180
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$tempVideo = Join-Path ([IO.Path]::GetTempPath()) ("video-share-e2e-{0}.mp4" -f [guid]::NewGuid().ToString("N"))

function Resolve-ApiUrl([string]$Value) {
    if ([Uri]::IsWellFormedUriString($Value, [UriKind]::Absolute)) {
        return $Value
    }
    return $BaseUrl.TrimEnd("/") + "/" + $Value.TrimStart("/")
}

function First-MediaLine([string]$Manifest) {
    return ($Manifest -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -and -not $_.StartsWith("#") } | Select-Object -First 1)
}

try {
    Write-Host "[1/8] Waiting for API readiness..."
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    $ready = $null
    do {
        try {
            $ready = Invoke-RestMethod -Uri "$BaseUrl/readyz" -TimeoutSec 5
            if ($ready.status -eq "ready") { break }
        } catch {
            Start-Sleep -Seconds 2
        }
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($null -eq $ready -or $ready.status -ne "ready") { throw "API did not become ready within 60 seconds" }

    Write-Host "[2/8] Generating a two-second MP4 fixture..."
    $localFFmpeg = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if ($localFFmpeg) {
        & $localFFmpeg.Source -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=640x360:rate=24" -f lavfi -i "anullsrc=r=44100:cl=stereo" -t 2 -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac $tempVideo
    } else {
        & docker exec video_share_worker ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=640x360:rate=24" -f lavfi -i "anullsrc=r=44100:cl=stereo" -t 2 -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac /tmp/stage3-e2e.mp4
        if ($LASTEXITCODE -ne 0) { throw "failed to generate fixture in video_share_worker" }
        & docker cp "video_share_worker:/tmp/stage3-e2e.mp4" $tempVideo
        if ($LASTEXITCODE -ne 0) { throw "failed to copy generated fixture from worker" }
    }
    if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed to generate fixture" }
    $file = Get-Item -LiteralPath $tempVideo

    Write-Host "[3/8] Registering and logging in a temporary user..."
    $suffix = [DateTime]::UtcNow.ToString("yyyyMMddHHmmssfff")
    $username = "stage3_$suffix"
    $password = ([guid]::NewGuid().ToString("N") + "Aa1!")
    $registerBody = @{ username = $username; password = $password; nickname = "Stage 3 E2E" } | ConvertTo-Json
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/register" -ContentType "application/json" -Body $registerBody | Out-Null
    $loginBody = @{ username = $username; password = $password } | ConvertTo-Json
    $login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/login" -ContentType "application/json" -Body $loginBody
    $headers = @{ Authorization = "Bearer $($login.data.access_token)" }

    Write-Host "[4/8] Creating a submission and uploading directly to MinIO..."
    $createBody = @{
        title = "Stage 3 E2E $suffix"
        description = "Automated asynchronous transcode acceptance test"
        file_name = $file.Name
        content_type = "video/mp4"
        file_size = $file.Length
    } | ConvertTo-Json
    $created = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/videos" -Headers $headers -ContentType "application/json" -Body $createBody
    $videoID = [uint64]$created.data.id
    if (-not $created.data.upload_url) { throw "create response did not include upload_url" }
    Invoke-WebRequest -Method Put -Uri $created.data.upload_url -InFile $tempVideo -ContentType "video/mp4" | Out-Null

    Write-Host "[5/8] Completing upload and entering asynchronous processing..."
    $completed = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/videos/$videoID/complete" -Headers $headers
    if ($completed.data.status -notin @("processing", "ready")) {
        throw "unexpected complete status: $($completed.data.status)"
    }

    Write-Host "[6/8] Polling worker progress..."
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    do {
        $owner = Invoke-RestMethod -Uri "$BaseUrl/api/v1/users/me/videos/$videoID" -Headers $headers
        $status = $owner.data.status
        Write-Host ("      status={0}, progress={1}" -f $status, $owner.data.processing_progress)
        if ($status -eq "ready") { break }
        if ($status -eq "failed") { throw "transcode failed: $($owner.data.processing_error)" }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($status -ne "ready") { throw "video did not become ready within $TimeoutSeconds seconds" }

    Write-Host "[7/8] Verifying HLS master, variant, segment and cover..."
    $detail = Invoke-RestMethod -Uri "$BaseUrl/api/v1/videos/$videoID"
    if ($detail.data.play_type -ne "hls") { throw "expected HLS playback, got $($detail.data.play_type)" }
    $master = (Invoke-WebRequest -Uri (Resolve-ApiUrl $detail.data.play_url)).Content
    if ($master -notmatch "#EXTM3U") { throw "invalid HLS master playlist" }
    $variantPath = First-MediaLine $master
    if (-not $variantPath) { throw "HLS master does not contain a variant" }
    $variant = (Invoke-WebRequest -Uri (Resolve-ApiUrl $variantPath)).Content
    if ($variant -notmatch "#EXTM3U") { throw "invalid HLS variant playlist" }
    $segmentPath = First-MediaLine $variant
    if (-not $segmentPath) { throw "HLS variant does not contain a segment" }
    $segment = Invoke-WebRequest -Uri (Resolve-ApiUrl $segmentPath)
    if ($segment.StatusCode -ne 200 -or $segment.RawContentLength -le 0) { throw "HLS segment is unavailable" }
    $cover = Invoke-WebRequest -Uri (Resolve-ApiUrl $detail.data.cover_url)
    if ($cover.StatusCode -ne 200 -or $cover.RawContentLength -le 0) { throw "cover is unavailable" }

    Write-Host "[8/8] Stage 3 E2E passed."
    Write-Host ("      video_id={0}, resolution={1}x{2}, duration_ms={3}" -f $videoID, $detail.data.width, $detail.data.height, $detail.data.duration_ms)
} finally {
    Remove-Item -LiteralPath $tempVideo -Force -ErrorAction SilentlyContinue
}
