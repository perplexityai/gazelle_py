Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Set-SmokeCommit {
    if ($env:BUILDKITE_PULL_REQUEST -and $env:BUILDKITE_PULL_REQUEST -ne 'false') {
        $merge = & buildkite-agent meta-data get github-pr-merge
        if ($LASTEXITCODE -ne 0 -or $merge -cnotmatch '^[0-9a-f]{40}$') { throw 'Missing pinned PR merge SHA' }
        & git fetch --no-tags origin $merge | Out-Host
        if ($LASTEXITCODE -ne 0) { throw 'Cannot fetch pinned PR merge' }
        $head = & git rev-parse 'FETCH_HEAD^2'
        if ($LASTEXITCODE -ne 0 -or $head -cne $env:BUILDKITE_COMMIT) { throw 'Pinned merge does not contain PR head' }
        & git checkout --detach FETCH_HEAD | Out-Host
        if ($LASTEXITCODE -ne 0) { throw 'Cannot check out pinned PR merge' }
    }
}

function Get-SmokeArtifacts {
    param([string]$Work, [string[]]$Files)
    foreach ($name in 'BUILDKITE_BUILD_ID', 'BUILDKITE_COMMIT', 'BUILDKITE_PIPELINE_SLUG', 'USE_BAZEL_VERSION', 'SMOKE_PRODUCER_STEP') {
        if (-not [Environment]::GetEnvironmentVariable($name)) { throw "Missing $name" }
    }
    $artifact = ".buildkite-artifacts/smoke/$env:USE_BAZEL_VERSION/windows-amd64"
    & buildkite-agent artifact download "$artifact/*" $Work --build $env:BUILDKITE_BUILD_ID --step $env:SMOKE_PRODUCER_STEP | Out-Host
    if ($LASTEXITCODE -ne 0) { throw 'Artifact download failed' }
    $directory = Join-Path $Work $artifact
    $marker = Get-Item -LiteralPath (Join-Path $directory 'manifest.json')
    if ($marker.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Unsafe artifact manifest' }
    $manifest = Get-Content -Raw -LiteralPath $marker.FullName | ConvertFrom-Json
    $expected = @{ commit = $env:BUILDKITE_COMMIT; build = $env:BUILDKITE_BUILD_ID; pipeline = $env:BUILDKITE_PIPELINE_SLUG; bazel = $env:USE_BAZEL_VERSION; platform = 'windows-amd64' }
    foreach ($key in $expected.Keys) {
        if ($manifest.$key -cne $expected[$key]) { throw "Artifact identity mismatch: $key" }
    }
    if (Compare-Object -CaseSensitive $Files @($manifest.files.PSObject.Properties.Name)) { throw 'Unexpected manifest files' }
    if (Compare-Object -CaseSensitive @($Files + 'manifest.json') @((Get-ChildItem -LiteralPath $directory).Name)) { throw 'Unexpected artifact files' }
    foreach ($name in $Files) {
        $file = Get-Item -LiteralPath (Join-Path $directory $name)
        if ($file.PSIsContainer -or ($file.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Unsafe artifact: $name" }
        if ((Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant() -cne $manifest.files.$name) { throw "Artifact checksum mismatch: $name" }
    }
    Write-Host 'Verified same-build Windows artifacts'
    return $directory
}
