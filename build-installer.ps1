param([string]$Version = '1.4.0')
$ErrorActionPreference = 'Stop'

& (Join-Path $PSScriptRoot 'build.ps1')

$compilerCandidates = @(
    $env:ISCC,
    (Join-Path $PSScriptRoot 'target\tools\InnoSetup\ISCC.exe'),
    (Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6\ISCC.exe'),
    (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe')
)
$compiler = $compilerCandidates | Where-Object { $_ -and (Test-Path -LiteralPath $_) } | Select-Object -First 1
if (-not $compiler) {
    $compilerCommand = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($compilerCommand) { $compiler = $compilerCommand.Source }
}
if (-not $compiler) { throw 'Inno Setup compiler not found. Set ISCC to the path of ISCC.exe.' }

& $compiler "/DAppVersion=$Version" (Join-Path $PSScriptRoot 'installer\ClipRecorder.iss')
if ($LASTEXITCODE -ne 0) { throw "ISCC failed (exit $LASTEXITCODE)" }
