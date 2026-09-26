#!/usr/bin/env bash
# Builds Cliff Crack for iOS into build-ios/<target>/CliffCrack.app, and
# optionally installs and starts it on the Simulator or a device.
#
#   renderer/  -> librenderer.a    (CMake + Ninja, CMAKE_SYSTEM_NAME=iOS)
#   cmd/game   -> libcliffcrack.a  (Go, -buildmode=c-archive, cgo through Xcode's clang)
#   ios/main.m + both libraries -> the app's executable (clang)
#
# Vulkan is MoltenVK, downloaded once into build-ios/deps and embedded as
# Frameworks/MoltenVK.framework. No Xcode project: like the Android build,
# the app bundle is put together by hand.
#
#   ./build-ios.sh --run                      # Simulator: build, install, start, follow the log
#   ./build-ios.sh --run --sim "iPad Air 11-inch (M3)"
#   ./build-ios.sh --run -- -mode run         # ... with the game's own flags after --
#   ./build-ios.sh --device --run             # your iPhone, over USB or Wi-Fi (see README)
#   ./build-ios.sh --device --bundle-id com.you.cliffcrack --run
#   ./build-ios.sh --device --sign <identity> --profile <.mobileprovision>   # choose them yourself
#
# For a device, the signing identity and provisioning profile are found by
# themselves (ios/find-signing.py) once Xcode has made them.
#
# Needs Xcode, Go, CMake, Ninja and glslc (brew install cmake ninja shaderc).
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
target=sim
config=Release
run=0
sim_name="iPhone 16 Pro"
identity=""
profile=""
server=""
bundle_id=""
moltenvk_version="v1.4.2"
min_ios="15.0"
game_args=()

while [[ $# -gt 0 ]]; do
    case "$1" in
    --device) target=device ;;
    --sim) sim_name="$2"; shift ;;
    --debug) config=Debug ;;
    --run) run=1 ;;
    --sign) identity="$2"; shift ;;
    --profile) profile="$2"; shift ;;
    --server) server="$2"; shift ;; # the online coordinator baked in (a phone has no command line)
    --bundle-id) bundle_id="$2"; shift ;;
    --) shift; game_args=("$@"); break ;;
    -h | --help) sed -n '2,26p' "$0"; exit 0 ;;
    *) echo "unknown option $1" >&2; exit 2 ;;
    esac
    shift
done

step() { printf '\033[36m==> %s\033[0m\n' "$*"; }

for tool in go cmake ninja glslc xcrun; do
    command -v "$tool" >/dev/null || { echo "$tool not found (brew install cmake ninja shaderc go)" >&2; exit 1; }
done

if [[ $target == sim ]]; then
    sdk=iphonesimulator
    triple="arm64-apple-ios${min_ios}-simulator"
    platform=iPhoneSimulator
    mvk_slice=ios-arm64_x86_64-simulator
else
    sdk=iphoneos
    triple="arm64-apple-ios${min_ios}"
    platform=iPhoneOS
    mvk_slice=ios-arm64
fi
build="$root/build-ios/$target"
if [[ $target == device && ( -z $identity || -z $profile ) ]]; then
    # Up front, so a missing profile fails before a minute of building.
    step "Find signing identity and profile"
    eval "$(python3 "$root/ios/find-signing.py" "$bundle_id")"
    echo "bundle $bundle_id, profile $(basename "$profile")"
fi
[[ -n $bundle_id ]] || bundle_id="com.cliffcrack.game"
sysroot="$(xcrun --sdk $sdk --show-sdk-path)"
mkdir -p "$build"

# ---- MoltenVK -----------------------------------------------------------------
deps="$root/build-ios/deps"
mvk="$deps/MoltenVK/MoltenVK/dynamic/MoltenVK.xcframework/$mvk_slice/MoltenVK.framework"
if [[ ! -d $mvk ]]; then
    step "Download MoltenVK $moltenvk_version"
    mkdir -p "$deps"
    curl -fL --progress-bar -o "$deps/MoltenVK-all.tar" \
        "https://github.com/KhronosGroup/MoltenVK/releases/download/$moltenvk_version/MoltenVK-all.tar"
    tar -xf "$deps/MoltenVK-all.tar" -C "$deps"
    rm "$deps/MoltenVK-all.tar"
fi

# ---- renderer -----------------------------------------------------------------
step "Configure renderer (iOS $target)"
cmake -S "$root/renderer" -B "$build/renderer" -G Ninja \
    -DCMAKE_SYSTEM_NAME=iOS -DCMAKE_OSX_SYSROOT=$sdk -DCMAKE_OSX_ARCHITECTURES=arm64 \
    -DCMAKE_OSX_DEPLOYMENT_TARGET=$min_ios -DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY \
    -DCMAKE_BUILD_TYPE=$config >/dev/null
step "Build renderer"
cmake --build "$build/renderer"

# ---- game (Go -> static library) ----------------------------------------------
# cgo compiles with Xcode's clang for the right SDK and target.
cc="$build/clang.sh"
printf '#!/bin/sh\nexec xcrun --sdk %s clang -target %s -isysroot "%s" "$@"\n' $sdk "$triple" "$sysroot" >"$cc"
chmod +x "$cc"
ldflags=""
[[ $config == Release ]] && ldflags="-s -w"
[[ -n $server ]] && ldflags+=" -X CliffCrack/game.DefaultServer=$server"
step "Build game (Go, c-archive)"
(cd "$root" && CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$cc" CXX="$cc" \
    go build -buildmode=c-archive -trimpath -ldflags="$ldflags" -o "$build/go/libcliffcrack.a" ./cmd/game)

# ---- app bundle ---------------------------------------------------------------
app="$build/CliffCrack.app"
step "Link $app"
rm -rf "$app"
mkdir -p "$app/shaders" "$app/Frameworks"
opt=-O2
[[ $config == Debug ]] && opt=-O0
xcrun --sdk $sdk clang -target "$triple" -isysroot "$sysroot" $opt -fobjc-arc \
    "$root/ios/main.m" \
    "$build/go/libcliffcrack.a" \
    "$build/renderer/bin/librenderer.a" \
    "$build/renderer/libimgui.a" \
    "$build/renderer/_deps/volk-build/libvolk.a" \
    "$build/renderer/_deps/vk_bootstrap-build/libvk-bootstrap.a" \
    -lc++ -lresolv \
    -framework UIKit -framework QuartzCore -framework CoreGraphics -framework Metal -framework Foundation \
    -framework CoreFoundation -framework CoreText -framework Security \
    -Wl,-rpath,@executable_path/Frameworks \
    -o "$app/CliffCrack"
sed -e "s/\$(BUNDLE_ID)/$bundle_id/" -e "s/\$(PLATFORM)/$platform/" "$root/ios/Info.plist" >"$app/Info.plist"
cp "$build/renderer/bin/shaders/"*.spv "$app/shaders/"
# The icon: actool turns the asset catalog into Assets.car and the icon
# files, and says what Info.plist needs to point at them.
xcrun actool "$root/ios/Assets.xcassets" --compile "$app" --platform $sdk \
    --minimum-deployment-target $min_ios --app-icon AppIcon --target-device iphone --target-device ipad \
    --output-partial-info-plist "$build/assets.plist" --output-format human-readable-text >/dev/null
/usr/libexec/PlistBuddy -c "Merge $build/assets.plist" "$app/Info.plist" >/dev/null
cp -R "$mvk" "$app/Frameworks/"

# ---- signing ------------------------------------------------------------------
if [[ $target == sim ]]; then
    step "Sign (ad hoc, for the Simulator)"
    codesign --force --sign - "$app/Frameworks/MoltenVK.framework"
    codesign --force --sign - "$app"
else
    step "Sign ($identity)"
    cp "$profile" "$app/embedded.mobileprovision"
    security cms -D -i "$profile" >"$build/profile.plist"
    /usr/libexec/PlistBuddy -x -c 'Print :Entitlements' "$build/profile.plist" >"$build/entitlements.plist"
    codesign --force --sign "$identity" "$app/Frameworks/MoltenVK.framework"
    codesign --force --sign "$identity" --entitlements "$build/entitlements.plist" "$app"
fi
printf '\033[32mBuilt %s\033[0m\n' "$app"

[[ $run == 1 ]] || exit 0

# ---- run ----------------------------------------------------------------------
if [[ $target == sim ]]; then
    udid="$(xcrun simctl list devices available | grep -F "$sim_name (" | head -1 | grep -oE '[0-9A-F-]{36}')" || true
    [[ -n $udid ]] || { echo "no simulator named \"$sim_name\" (xcrun simctl list devices)" >&2; exit 1; }
    step "Boot $sim_name"
    xcrun simctl boot "$udid" 2>/dev/null || true
    open -a Simulator --args -CurrentDeviceUDID "$udid"
    step "Install and start"
    xcrun simctl install "$udid" "$app"
    exec xcrun simctl launch --console-pty --terminate-running-process "$udid" "$bundle_id" ${game_args[@]+"${game_args[@]}"}
else
    step "Install and start (devicectl)"
    xcrun devicectl list devices --json-output "$build/devices.json" >/dev/null 2>&1 || true
    device="$(python3 -c '
import json, sys
for d in json.load(open(sys.argv[1]))["result"]["devices"]:
    if d["hardwareProperties"].get("platform") == "iOS" and d["connectionProperties"].get("pairingState") == "paired":
        print(d["identifier"]); break
' "$build/devices.json" 2>/dev/null)" || true
    [[ -n $device ]] || { echo "no paired iPhone found: plug it in, unlock it and trust this Mac (xcrun devicectl list devices)" >&2; exit 1; }
    xcrun devicectl device install app --device "$device" "$app"
    exec xcrun devicectl device process launch --console --terminate-existing --device "$device" "$bundle_id" ${game_args[@]+"${game_args[@]}"}
fi
