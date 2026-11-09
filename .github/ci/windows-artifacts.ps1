Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'


function Get-SmokeArtifacts {
    param([string]$Work, [string[]]$Files)
    foreach ($name in 'CI_RUN_ID', 'GITHUB_SHA', 'GITHUB_REPOSITORY', 'USE_BAZEL_VERSION') {
        if (-not [Environment]::GetEnvironmentVariable($name)) { throw "Missing $name" }
    }
    $artifact = ".ci-artifacts/smoke/$env:USE_BAZEL_VERSION/windows-amd64"
    Copy-Item -Recurse -LiteralPath (Join-Path $PWD ".ci-artifacts") -Destination $Work
    $directory = Join-Path $Work $artifact
    $marker = Get-Item -LiteralPath (Join-Path $directory 'manifest.json')
    if ($marker.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Unsafe artifact manifest' }
    $manifest = Get-Content -Raw -LiteralPath $marker.FullName | ConvertFrom-Json
    $expected = @{ commit = $env:GITHUB_SHA; build = $env:CI_RUN_ID; pipeline = $env:GITHUB_REPOSITORY; bazel = $env:USE_BAZEL_VERSION; platform = 'windows-amd64' }
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
