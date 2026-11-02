$ErrorActionPreference = 'Stop'

# Builds the ClipSync Android app locally and puts the APK next to the Windows
# builds. Needs JDK 17, an Android SDK, and Gradle 8.7 unpacked under
# target\tools\gradle-8.7 (the CI workflow downloads it on its own runner).

$root = (Resolve-Path -LiteralPath $PSScriptRoot).Path
$gradle = Join-Path $root 'target\tools\gradle-8.7\bin\gradle.bat'
if (-not (Test-Path -LiteralPath $gradle)) {
    throw "Gradle 8.7 not found at $gradle. Download gradle-8.7-bin.zip into target\tools and unpack it there."
}

if (-not $env:JAVA_HOME) {
    $candidate = 'C:\Program Files\Java\jdk-17'
    if (Test-Path -LiteralPath (Join-Path $candidate 'bin\javac.exe')) {
        $env:JAVA_HOME = $candidate
    }
}
if (-not $env:JAVA_HOME) {
    throw 'JAVA_HOME is not set and no JDK 17 was found.'
}

$sdk = Join-Path $root 'android\local.properties'
if (-not (Test-Path -LiteralPath $sdk)) {
    throw "android\local.properties is missing; it must contain sdk.dir=<path to the Android SDK>"
}

& $gradle -p (Join-Path $root 'android') :app:assembleDebug --no-daemon
if ($LASTEXITCODE -ne 0) {
    throw "gradle failed (exit $LASTEXITCODE)"
}

$apk = Join-Path $root 'android\app\build\outputs\apk\debug\app-debug.apk'
if (-not (Test-Path -LiteralPath $apk)) {
    throw "APK not found at $apk"
}
$target = Join-Path $root 'dist\ClipSync-debug.apk'
Copy-Item -LiteralPath $apk -Destination $target -Force
Write-Output $apk
Write-Output $target
