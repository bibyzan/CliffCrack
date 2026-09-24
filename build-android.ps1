<#
.SYNOPSIS
    Builds Cliff Crack for Android (arm64) into build-android/CliffCrack.apk,
    and optionally installs and starts it on a device over adb.
.DESCRIPTION
    renderer/  -> librenderer.so   (CMake + Ninja with the NDK's toolchain)
    cmd/game   -> libcliffcrack.so (Go, -buildmode=c-shared, cgo through the NDK's clang)
    Both go into an APK with the compiled shaders as assets, packaged with
    aapt2, zipalign and apksigner (debug key) from the Android SDK: no Gradle.
.EXAMPLE
    ./build-android.ps1 -Run          # build, install on the connected device, start
    ./build-android.ps1 -Run -Log     # ... then follow the game's log
#>
param(
    [ValidateSet('Debug', 'Release')]
    [string] $Config = 'Release',
    [switch] $Install,
    [switch] $Run,
    [switch] $Log
)

$ErrorActionPreference = 'Stop'
$root  = $PSScriptRoot
$build = Join-Path $root 'build-android'
$minApi = 29 # Android 10

function Invoke-Step([string] $Name, [scriptblock] $Command) {
    Write-Host "==> $Name" -ForegroundColor Cyan
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "$Name failed (exit code $LASTEXITCODE)" }
}

# Newest subdirectory of $dir (NDK, build-tools and platform versions).
function Get-Newest([string] $dir, [string] $filter = '*') {
    $found = Get-ChildItem $dir -Directory -Filter $filter -ErrorAction SilentlyContinue |
        Sort-Object {
            $v = $_.Name -replace '^android-', '' -replace '[^0-9.].*$', ''
            if ($v -notmatch '\.') { $v += '.0' } # [version] needs major.minor
            [version]$v
        } |
        Select-Object -Last 1
    if (-not $found) { throw "nothing in $dir - install it with the Android SDK manager" }
    return $found.FullName
}

# ---- toolchain ---------------------------------------------------------------
$sdk = @($env:ANDROID_HOME, $env:ANDROID_SDK_ROOT, "$env:LOCALAPPDATA\Android\Sdk", "$env:USERPROFILE\Android\Sdk") |
    Where-Object { $_ -and (Test-Path $_) } | Select-Object -First 1
if (-not $sdk) { throw 'Android SDK not found: set ANDROID_HOME' }
$ndk        = Get-Newest (Join-Path $sdk 'ndk')
$buildTools = Get-Newest (Join-Path $sdk 'build-tools')
$platform   = Get-Newest (Join-Path $sdk 'platforms') 'android-*'
$adb        = Join-Path $sdk 'platform-tools\adb.exe'
$clangBin   = Join-Path $ndk 'toolchains\llvm\prebuilt\windows-x86_64\bin'
Write-Host "SDK $sdk`nNDK $ndk`nbuild-tools $buildTools`nplatform $platform"

# ---- renderer ----------------------------------------------------------------
Invoke-Step 'Configure renderer (Android arm64)' {
    cmake -S (Join-Path $root 'renderer') -B $build -G Ninja `
        "-DCMAKE_TOOLCHAIN_FILE=$ndk/build/cmake/android.toolchain.cmake" `
        -DANDROID_ABI=arm64-v8a "-DANDROID_PLATFORM=android-$minApi" -DANDROID_STL=c++_static `
        "-DCMAKE_BUILD_TYPE=$Config"
}
Invoke-Step 'Build renderer' { cmake --build $build }

# ---- game (Go -> shared library) -----------------------------------------------
$apkDir = Join-Path $build 'apk'
$libDir = Join-Path $apkDir 'lib\arm64-v8a'
$assetDir = Join-Path $apkDir 'assets'
Remove-Item $apkDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $libDir, (Join-Path $assetDir 'shaders') | Out-Null

Push-Location $root
try {
    $env:CGO_ENABLED = '1'
    $env:GOOS = 'android'
    $env:GOARCH = 'arm64'
    $env:CC = Join-Path $clangBin "aarch64-linux-android$minApi-clang.cmd"
    $env:CXX = Join-Path $clangBin "aarch64-linux-android$minApi-clang++.cmd"
    $ldflags = if ($Config -eq 'Release') { '-s -w' } else { '' }
    Invoke-Step 'Build game (Go, c-shared)' {
        go build -buildmode=c-shared -trimpath "-ldflags=$ldflags" -o (Join-Path $libDir 'libcliffcrack.so') ./cmd/game
    }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CC, Env:CXX -ErrorAction SilentlyContinue
    Pop-Location
}
Remove-Item (Join-Path $libDir 'libcliffcrack.h') -ErrorAction SilentlyContinue # cgo's export header
Copy-Item (Join-Path $build 'bin\librenderer.so') $libDir
Copy-Item (Join-Path $build 'bin\shaders\*.spv') (Join-Path $assetDir 'shaders')

# ---- package -----------------------------------------------------------------
$unaligned = Join-Path $build 'CliffCrack.unaligned.apk'
$aligned   = Join-Path $build 'CliffCrack.aligned.apk'
$apk       = Join-Path $build 'CliffCrack.apk'
$aaptArgs = @('link', '-o', $unaligned, '--manifest', (Join-Path $root 'android\AndroidManifest.xml'),
    '-I', (Join-Path $platform 'android.jar'),
    '--min-sdk-version', $minApi, '--target-sdk-version', 34, '--version-code', 1, '--version-name', '0.1')
if ($Config -eq 'Debug') { $aaptArgs += '--debug-mode' }
Invoke-Step 'Package (aapt2)' { & (Join-Path $buildTools 'aapt2.exe') @aaptArgs }

# Assets and native libraries are added here rather than by aapt2, whose -A
# writes nested asset paths with backslashes on Windows (Android then can't
# find them). The manifest sets extractNativeLibs, so the libraries may be
# compressed: Android unpacks them at install time.
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [System.IO.Compression.ZipFile]::Open($unaligned, 'Update')
try {
    foreach ($file in Get-ChildItem $apkDir -Recurse -File) {
        $name = $file.FullName.Substring($apkDir.Length + 1).Replace('\', '/')
        [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $file.FullName, $name) | Out-Null
    }
} finally {
    $zip.Dispose()
}
Invoke-Step 'Align (zipalign)' { & (Join-Path $buildTools 'zipalign.exe') -f -p 4 $unaligned $aligned }

$keystore = Join-Path $env:USERPROFILE '.android\debug.keystore'
if (-not (Test-Path $keystore)) {
    New-Item -ItemType Directory -Force (Split-Path $keystore) | Out-Null
    Invoke-Step 'Create debug keystore' {
        keytool -genkeypair -keystore $keystore -alias androiddebugkey -storepass android -keypass android `
            -keyalg RSA -keysize 2048 -validity 10000 -dname 'CN=Android Debug,O=Android,C=US'
    }
}
Invoke-Step 'Sign (apksigner, debug key)' {
    & (Join-Path $buildTools 'apksigner.bat') sign --ks $keystore --ks-pass pass:android --key-pass pass:android `
        --out $apk $aligned
}
Remove-Item $unaligned, $aligned -ErrorAction SilentlyContinue
Write-Host "Built $apk" -ForegroundColor Green

# ---- device ------------------------------------------------------------------
if ($Install -or $Run -or $Log) {
    Invoke-Step 'Install (adb)' { & $adb install -r $apk }
}
if ($Run -or $Log) {
    & $adb logcat -c
    Invoke-Step 'Start' { & $adb shell am start -n com.cliffcrack.game/android.app.NativeActivity }
}
if ($Log) {
    & $adb logcat -s CliffCrack:V Vulkan:W DEBUG:E AndroidRuntime:E
}
