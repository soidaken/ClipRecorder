$ErrorActionPreference = 'Stop'

# Stop only ClipRecorder executables belonging to this project before replacing
# the standard distribution binary.
$projectRoot = (Resolve-Path -LiteralPath $PSScriptRoot).Path
$projectPrefix = $projectRoot.TrimEnd('\') + '\'
Get-Process | Where-Object {
    $_.ProcessName -like 'ClipRecorder*' -and $_.Path -and
    $_.Path.StartsWith($projectPrefix, [System.StringComparison]::OrdinalIgnoreCase)
} | ForEach-Object {
    Stop-Process -Id $_.Id -Force
    Wait-Process -Id $_.Id -ErrorAction SilentlyContinue
}

Push-Location -LiteralPath $projectRoot
try {
    go build -trimpath -ldflags '-s -w -H windowsgui' -o dist/ClipRecorder.exe .
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed (exit $LASTEXITCODE)"
    }
    Write-Output (Join-Path $projectRoot 'dist\ClipRecorder.exe')
} finally {
    Pop-Location
}
