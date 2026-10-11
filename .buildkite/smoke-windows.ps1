. "$PSScriptRoot/windows-artifacts.ps1"
Set-SmokeCommit
$work = Join-Path $env:TEMP ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    $directory = Get-SmokeArtifacts -Work $work -Files @('gazelle.exe', 'unit-test.exe')
    $env:TEST_TMPDIR = Join-Path $work 'tests'
    New-Item -ItemType Directory -Path $env:TEST_TMPDIR | Out-Null
    & (Join-Path $directory 'unit-test.exe') '-test.v'
    if ($LASTEXITCODE -ne 0) { throw 'Unit tests failed' }
    $fixture = Join-Path $work 'fixture'
    New-Item -ItemType Directory -Path $fixture | Out-Null
    [IO.File]::WriteAllText((Join-Path $fixture 'MODULE.bazel'), 'module(name = "smoke")' + "`n")
    [IO.File]::WriteAllText((Join-Path $fixture 'main.py'), 'import os' + "`n")
    & (Join-Path $directory 'gazelle.exe') "-repo_root=$fixture" $fixture
    if ($LASTEXITCODE -ne 0) { throw 'Gazelle generation failed' }
    if (-not (Select-String -LiteralPath (Join-Path $fixture 'BUILD.bazel') -SimpleMatch 'py_library(')) { throw 'Missing generated rule' }
    & (Join-Path $directory 'gazelle.exe') "-repo_root=$fixture" '-mode=diff' $fixture
    if ($LASTEXITCODE -ne 0) { throw 'Gazelle output drifted' }
} finally {
    Remove-Item -Recurse -Force -LiteralPath $work
}
