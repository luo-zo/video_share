# 第四阶段社区互动验收脚本。
#
# 覆盖公开搜索、播放清单、点赞 / 收藏幂等、评论、观看历史、关注关系、作者编辑与
# 删除，以及所有可逆关系的取消。任何非预期状态码或字段值都会立即失败。
#
# 脚本只创建和操作自己生成的数据：注册两个全新的测试账号，由作者账号现场生成、
# 上传并转码一支短视频，结束时删除该视频。它永远不会读写已有用户的投稿。
param(
    [string]$BaseUrl = "http://127.0.0.1:8081",
    [int]$TimeoutSeconds = 180
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$tempVideo = Join-Path ([IO.Path]::GetTempPath()) ("video-share-stage4-{0}.mp4" -f [guid]::NewGuid().ToString("N"))
$author = $null
$video = $null
$cleanupVideoID = [uint64]0
$cleanupVideoHeaders = $null
$skipHttpErrorCheck = $PSVersionTable.PSVersion.Major -ge 6

function Resolve-ApiUrl([string]$Value) {
    if ([Uri]::IsWellFormedUriString($Value, [UriKind]::Absolute)) {
        return $Value
    }
    return $BaseUrl.TrimEnd("/") + "/" + $Value.TrimStart("/")
}

function Assert-True($Condition, [string]$Label) {
    if (-not $Condition) { throw "assertion failed: $Label" }
}

function Assert-Equal($Actual, $Expected, [string]$Label) {
    if ($Actual -ne $Expected) {
        throw ("assertion failed: {0} (got '{1}', want '{2}')" -f $Label, $Actual, $Expected)
    }
}

# 统一发起请求并返回 @{status, data, raw}。非预期状态直接抛错，便于把断言写在调用点。
function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers,
        $Body,
        [int[]]$ExpectStatus = @(200)
    )
    $params = @{ Method = $Method; Uri = (Resolve-ApiUrl $Path); UseBasicParsing = $true }
    if ($Headers) { $params.Headers = $Headers }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 6)
    }
    if ($skipHttpErrorCheck) { $params.SkipHttpErrorCheck = $true }

    $status = 0
    $content = ""
    try {
        $response = Invoke-WebRequest @params
        $status = [int]$response.StatusCode
        $content = $response.Content
        # PowerShell 7 会把 application/vnd.apple.mpegurl 响应保留为 byte[]；
        # 统一解码成 UTF-8 文本，后面的 JSON 解析与 HLS 断言才能得到真实正文。
        if ($content -is [byte[]]) {
            $content = [Text.Encoding]::UTF8.GetString($content)
        }
    } catch {
        $webResponse = $_.Exception.Response
        if ($null -eq $webResponse) { throw }
        $status = [int]$webResponse.StatusCode
        $reader = New-Object System.IO.StreamReader($webResponse.GetResponseStream())
        $content = $reader.ReadToEnd()
        $reader.Dispose()
    }
    if ($ExpectStatus -notcontains $status) {
        throw ("{0} {1} returned {2}, expected {3}. Body: {4}" -f $Method, $Path, $status, ($ExpectStatus -join "/"), $content)
    }
    # HLS 清单等非 JSON 响应没有可解析的负载，保留原始文本供调用点断言。
    $parsed = $null
    if (-not [string]::IsNullOrWhiteSpace($content)) {
        try { $parsed = $content | ConvertFrom-Json } catch { $parsed = $null }
    }
    return @{ status = $status; data = $parsed; raw = $content }
}

function New-TestUser([string]$Prefix, [string]$Nickname) {
    # users.username 最长 32 字符；12 位时间戳加 4 位随机串仍给当前前缀留足空间。
    $suffix = [DateTime]::UtcNow.ToString("yyMMddHHmmss") + ([guid]::NewGuid().ToString("N").Substring(0, 4))
    $username = "{0}_{1}" -f $Prefix, $suffix
    $password = [guid]::NewGuid().ToString("N") + "Aa1!"
    Invoke-Api -Method Post -Path "/api/v1/auth/register" -ExpectStatus 201 -Body @{
        username = $username; password = $password; nickname = $Nickname
    } | Out-Null
    $login = Invoke-Api -Method Post -Path "/api/v1/auth/login" -Body @{ username = $username; password = $password }
    $token = $login.data.data.access_token
    Assert-True ($token -is [string] -and $token.Length -gt 0) "login returns access_token"
    $headers = @{ Authorization = "Bearer $token" }
    $me = Invoke-Api -Method Get -Path "/api/v1/users/me" -Headers $headers
    return @{ username = $username; id = [uint64]$me.data.data.id; headers = $headers }
}

function New-TestVideo([hashtable]$Author) {
    Write-Host "      生成两秒钟的 MP4 测试素材..."
    $localFFmpeg = Get-Command ffmpeg -ErrorAction SilentlyContinue
    if ($localFFmpeg) {
        & $localFFmpeg.Source -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=640x360:rate=24" -f lavfi -i "anullsrc=r=44100:cl=stereo" -t 2 -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac $tempVideo
    } else {
        & docker exec video_share_worker ffmpeg -hide_banner -loglevel error -y -f lavfi -i "testsrc2=size=640x360:rate=24" -f lavfi -i "anullsrc=r=44100:cl=stereo" -t 2 -shortest -c:v libx264 -pix_fmt yuv420p -c:a aac /tmp/stage4-e2e.mp4
        if ($LASTEXITCODE -ne 0) { throw "failed to generate fixture in video_share_worker" }
        & docker cp "video_share_worker:/tmp/stage4-e2e.mp4" $tempVideo
        if ($LASTEXITCODE -ne 0) { throw "failed to copy generated fixture from worker" }
    }
    if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed to generate fixture" }
    $file = Get-Item -LiteralPath $tempVideo

    $title = "Stage 4 E2E " + [DateTime]::UtcNow.ToString("yyyyMMddHHmmssfff")
    $created = Invoke-Api -Method Post -Path "/api/v1/videos" -Headers $Author.headers -ExpectStatus 201 -Body @{
        title = $title
        description = "Automated community interaction acceptance test"
        file_name = $file.Name
        content_type = "video/mp4"
        file_size = $file.Length
    }
    $id = [uint64]$created.data.data.id
    $script:cleanupVideoID = $id
    $script:cleanupVideoHeaders = $Author.headers
    Assert-True ($created.data.data.upload_url.Length -gt 0) "create returns upload_url"
    Invoke-WebRequest -Method Put -Uri $created.data.data.upload_url -InFile $tempVideo -ContentType "video/mp4" -UseBasicParsing | Out-Null
    Invoke-Api -Method Post -Path "/api/v1/videos/$id/complete" -Headers $Author.headers -ExpectStatus 202 | Out-Null

    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $status = "processing"
    do {
        $owner = Invoke-Api -Method Get -Path "/api/v1/users/me/videos/$id" -Headers $Author.headers
        $status = $owner.data.data.status
        if ($status -eq "failed") { throw "transcode failed: $($owner.data.data.processing_error)" }
        if ($status -eq "ready") { break }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    if ($status -ne "ready") { throw "video did not become ready within $TimeoutSeconds seconds" }
    return @{ id = $id; headers = $Author.headers; title = $title }
}

function Find-Video([uint64]$Target, [string]$Query, [hashtable]$Headers = $null) {
    $path = "/api/v1/videos?q={0}&page_size=50" -f [Uri]::EscapeDataString($Query)
    $result = Invoke-Api -Method Get -Path $path -Headers $Headers
    return @($result.data.data.items | Where-Object { [uint64]$_.id -eq $Target })
}

try {
    Write-Host "[1/11] 等待 API 就绪..."
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

    Write-Host "[2/11] 创建作者账号并准备一支 ready 的公开视频..."
    $author = New-TestUser "stage4_author" "Stage 4 Author"
    $video = New-TestVideo $author

    Write-Host "[3/11] 创建观众账号..."
    $viewer = New-TestUser "stage4_viewer" "Stage 4 Viewer"
    Assert-True ($viewer.id -ne $author.id) "两个测试账号必须不同"

    Write-Host "[4/11] 匿名播放、匿名搜索命中，并验证 LIKE 通配符被转义..."
    $anonymous = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)"
    Assert-True (-not ($anonymous.data.data.PSObject.Properties.Name -contains "viewer_state")) "匿名详情不应包含 viewer_state"
    Assert-Equal $anonymous.data.data.play_type "hls" "ready 视频应提供 HLS 播放"
    Assert-Equal $anonymous.data.data.play_url "/api/v1/videos/$($video.id)/hls/master.m3u8" "play_url 应指向清单代理"
    $manifest = Invoke-Api -Method Get -Path $anonymous.data.data.play_url
    Assert-True ($manifest.raw -like "*#EXTM3U*") "匿名请求应能取到 HLS 清单"
    Assert-Equal $anonymous.data.data.stats.like_count 0 "初始点赞数为 0"
    Assert-Equal $anonymous.data.data.stats.favorite_count 0 "初始收藏数为 0"
    Assert-Equal $anonymous.data.data.stats.comment_count 0 "初始评论数为 0"
    $viewsBefore = [uint64]$anonymous.data.data.stats.view_count

    $found = Find-Video -Target $video.id -Query $video.title
    Assert-Equal $found.Count 1 "按标题搜索应命中该视频"
    Assert-Equal ([uint64]$found[0].author.id) $author.id "搜索结果作者应与上传者一致"
    Assert-True (-not ($found[0].PSObject.Properties.Name -contains "viewer_state")) "匿名搜索不应包含 viewer_state"
    Assert-Equal (Find-Video -Target $video.id -Query "%").Count 0 "百分号必须按字面匹配"
    Assert-Equal (Find-Video -Target $video.id -Query "_tage4").Count 0 "下划线必须按字面匹配"

    Write-Host "[5/11] 验证点赞与收藏的幂等写入..."
    $like = Invoke-Api -Method Put -Path "/api/v1/videos/$($video.id)/like" -Headers $viewer.headers
    Assert-Equal $like.data.data.active $true "点赞后 active 应为 true"
    Assert-Equal ([uint64]$like.data.data.count) 1 "点赞后计数应为 1"
    $likeAgain = Invoke-Api -Method Put -Path "/api/v1/videos/$($video.id)/like" -Headers $viewer.headers
    Assert-Equal ([uint64]$likeAgain.data.data.count) 1 "重复点赞不应增加计数"
    $favorite = Invoke-Api -Method Put -Path "/api/v1/videos/$($video.id)/favorite" -Headers $viewer.headers
    Assert-Equal $favorite.data.data.active $true "收藏后 active 应为 true"
    Assert-Equal ([uint64]$favorite.data.data.count) 1 "收藏后计数应为 1"
    $favoriteAgain = Invoke-Api -Method Put -Path "/api/v1/videos/$($video.id)/favorite" -Headers $viewer.headers
    Assert-Equal ([uint64]$favoriteAgain.data.data.count) 1 "重复收藏不应增加计数"

    Write-Host "[6/11] 发表评论并读取评论列表..."
    $content = "stage4 comment " + [guid]::NewGuid().ToString("N").Substring(0, 8)
    $created = Invoke-Api -Method Post -Path "/api/v1/videos/$($video.id)/comments" -Headers $viewer.headers -ExpectStatus 201 -Body @{ content = $content }
    $commentID = [uint64]$created.data.data.id
    Assert-True ($commentID -gt 0) "发表评论应返回评论 ID"
    $comments = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)/comments"
    Assert-Equal ([int]$comments.data.data.total) 1 "评论总数应为 1"
    Assert-Equal $comments.data.data.items[0].content $content "评论内容应与提交一致"
    Assert-Equal ([uint64]$comments.data.data.items[0].user_id) $viewer.id "评论作者应为观众账号"
    Invoke-Api -Method Post -Path "/api/v1/videos/$($video.id)/comments" -Headers $viewer.headers -ExpectStatus 400 -Body @{ content = "   " } | Out-Null

    Write-Host "[7/11] 上报观看进度并验证历史与播放计数..."
    Invoke-Api -Method Post -Path "/api/v1/videos/$($video.id)/watch" -Headers $viewer.headers -Body @{ progress_ms = 1200; duration_ms = 2000 } | Out-Null
    $history = Invoke-Api -Method Get -Path "/api/v1/users/me/history" -Headers $viewer.headers
    Assert-True (@($history.data.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "观看历史应包含该视频"
    Invoke-Api -Method Post -Path "/api/v1/videos/$($video.id)/watch" -Headers $viewer.headers -Body @{ progress_ms = 1900; duration_ms = 2000 } | Out-Null
    $historyAgain = Invoke-Api -Method Get -Path "/api/v1/users/me/history" -Headers $viewer.headers
    Assert-True (@($historyAgain.data.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "重复上报不应产生第二条历史"
    $afterWatch = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)"
    Assert-Equal ([uint64]$afterWatch.data.data.stats.view_count) ($viewsBefore + 1) "同一用户重复观看只增加一次播放数"
    Invoke-Api -Method Post -Path "/api/v1/videos/$($video.id)/watch" -Headers $viewer.headers -ExpectStatus 400 -Body @{ progress_ms = 2000; duration_ms = 1000 } | Out-Null

    Write-Host "[8/11] 验证关注、自关注拒绝与关注列表..."
    $follow = Invoke-Api -Method Put -Path "/api/v1/users/$($author.id)/follow" -Headers $viewer.headers
    Assert-Equal $follow.data.data.following $true "关注后 following 应为 true"
    $followAgain = Invoke-Api -Method Put -Path "/api/v1/users/$($author.id)/follow" -Headers $viewer.headers
    Assert-Equal $followAgain.data.data.following $true "重复关注应为幂等"
    $follows = Invoke-Api -Method Get -Path "/api/v1/users/me/follows" -Headers $viewer.headers
    Assert-True (@($follows.data.data.items | Where-Object { [uint64]$_.id -eq $author.id }).Count -eq 1) "关注列表应包含作者"
    $self = Invoke-Api -Method Put -Path "/api/v1/users/$($viewer.id)/follow" -Headers $viewer.headers -ExpectStatus 400
    Assert-Equal $self.data.error.code "SELF_FOLLOW" "自关注应返回 SELF_FOLLOW"

    Write-Host "[9/11] 验证带令牌的详情返回 viewer_state 与最终计数..."
    $detail = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)" -Headers $viewer.headers
    Assert-Equal $detail.data.data.viewer_state.liked $true "viewer_state.liked 应为 true"
    Assert-Equal $detail.data.data.viewer_state.favorited $true "viewer_state.favorited 应为 true"
    Assert-Equal $detail.data.data.viewer_state.following_author $true "viewer_state.following_author 应为 true"
    Assert-Equal ([uint64]$detail.data.data.stats.like_count) 1 "点赞计数应为 1"
    Assert-Equal ([uint64]$detail.data.data.stats.favorite_count) 1 "收藏计数应为 1"
    Assert-Equal ([uint64]$detail.data.data.stats.comment_count) 1 "评论计数应为 1"
    $favorites = Invoke-Api -Method Get -Path "/api/v1/users/me/favorites" -Headers $viewer.headers
    Assert-True (@($favorites.data.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "收藏列表应包含该视频"
    $mine = Invoke-Api -Method Get -Path "/api/v1/users/me/videos" -Headers $video.headers
    Assert-True (@($mine.data.data.items | Where-Object { [uint64]$_.id -eq $video.id }).Count -eq 1) "作者投稿列表应包含该视频"

    Write-Host "[10/11] 验证作者编辑标题、简介与可见性..."
    $patched = Invoke-Api -Method Patch -Path "/api/v1/users/me/videos/$($video.id)" -Headers $video.headers -Body @{
        title = "$($video.title) (编辑)"; description = "updated by e2e"; visibility = "private"
    }
    Assert-Equal $patched.data.data.visibility "private" "可见性应更新为 private"
    Assert-Equal $patched.data.data.title "$($video.title) (编辑)" "标题应被更新"
    Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)" -ExpectStatus 404 | Out-Null
    Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)/hls/master.m3u8" -ExpectStatus 404 | Out-Null
    Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)/cover" -ExpectStatus 404 | Out-Null
    Assert-Equal (Find-Video -Target $video.id -Query $video.title).Count 0 "私有视频不应出现在公开搜索中"
    $restored = Invoke-Api -Method Patch -Path "/api/v1/users/me/videos/$($video.id)" -Headers $video.headers -Body @{ visibility = "public" }
    Assert-Equal $restored.data.data.visibility "public" "可见性应恢复为 public"
    Assert-Equal (Find-Video -Target $video.id -Query $patched.data.data.title).Count 1 "恢复公开后应可再次被搜索到"
    Invoke-Api -Method Patch -Path "/api/v1/users/me/videos/$($video.id)" -Headers $viewer.headers -Body @{
        title = "forbidden edit"
    } -ExpectStatus 403 | Out-Null

    Write-Host "[11/11] 取消全部可逆关系、复核计数并清理..."
    $unlike = Invoke-Api -Method Delete -Path "/api/v1/videos/$($video.id)/like" -Headers $viewer.headers
    Assert-Equal $unlike.data.data.active $false "取消点赞后 active 应为 false"
    Assert-Equal ([uint64]$unlike.data.data.count) 0 "取消点赞后计数应归零"
    $unfavorite = Invoke-Api -Method Delete -Path "/api/v1/videos/$($video.id)/favorite" -Headers $viewer.headers
    Assert-Equal ([uint64]$unfavorite.data.data.count) 0 "取消收藏后计数应归零"
    Invoke-Api -Method Delete -Path "/api/v1/comments/$commentID" -Headers $viewer.headers | Out-Null
    $afterDeleteComment = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)/comments"
    Assert-Equal ([int]$afterDeleteComment.data.data.total) 0 "删除评论后总数应为 0"
    $unfollow = Invoke-Api -Method Delete -Path "/api/v1/users/$($author.id)/follow" -Headers $viewer.headers
    Assert-Equal $unfollow.data.data.following $false "取关后 following 应为 false"
    $final = Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)"
    Assert-Equal ([uint64]$final.data.data.stats.like_count) 0 "最终点赞计数应为 0"
    Assert-Equal ([uint64]$final.data.data.stats.favorite_count) 0 "最终收藏计数应为 0"
    Assert-Equal ([uint64]$final.data.data.stats.comment_count) 0 "最终评论计数应为 0"
    $emptyFavorites = Invoke-Api -Method Get -Path "/api/v1/users/me/favorites" -Headers $viewer.headers
    Assert-Equal ([int]$emptyFavorites.data.data.total) 0 "取消收藏后收藏列表应为空"

    Invoke-Api -Method Delete -Path "/api/v1/users/me/videos/$($video.id)" -Headers $video.headers | Out-Null
    $cleanupVideoID = [uint64]0
    $cleanupVideoHeaders = $null
    Invoke-Api -Method Get -Path "/api/v1/videos/$($video.id)" -ExpectStatus 404 | Out-Null
    Assert-Equal (Find-Video -Target $video.id -Query $video.title).Count 0 "已删除视频不应出现在公开搜索中"

    Write-Host ""
    Write-Host "第四阶段社区互动验收通过。"
    Write-Host ("      video_id={0}, author_id={1}, viewer_id={2}" -f $video.id, $author.id, $viewer.id)
} finally {
    # 无论成功还是中途失败，都尽力软删除本次创建的投稿，避免它继续出现在公开视频列表中。
    if ($cleanupVideoID -ne 0 -and $null -ne $cleanupVideoHeaders) {
        try {
            Invoke-Api -Method Delete -Path "/api/v1/users/me/videos/$cleanupVideoID" -Headers $cleanupVideoHeaders -ExpectStatus @(200, 404) | Out-Null
            Write-Host "      已清理本次创建的测试视频 video_id=$cleanupVideoID"
        } catch {
            Write-Warning ("无法清理本次创建的视频 {0}: {1}" -f $cleanupVideoID, $_.Exception.Message)
        }
    }
    Remove-Item -LiteralPath $tempVideo -Force -ErrorAction SilentlyContinue
}
