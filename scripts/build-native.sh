#!/usr/bin/env bash
# Linux equivalent of build-native.ps1: four obfuscated Android ABIs of libdtpsecure.so.
# Usage: scripts/build-native.sh <NDK 23.1 path> <xollvm v0.12.0 LLVM22 path>
set -euo pipefail
ndk=${1:?NDK path required}
obfuscator=${2:?xollvm path required}
repo=$(cd "$(dirname "$0")/.." && pwd)
source=$repo/android/sdk-secure/src/main/cpp/frame.cpp
output=$repo/android/sdk-secure/src/main/jniLibs
build=$repo/work/native
stock=$ndk/toolchains/llvm/prebuilt/linux-x86_64
clang=$obfuscator/bin/clang++
opt=$obfuscator/bin/opt
mkdir -p "$build"
manifest="["
for spec in arm64-v8a:aarch64-linux-android24:aarch64-linux-android \
            armeabi-v7a:armv7a-linux-androideabi24:arm-linux-androideabi \
            x86_64:x86_64-linux-android24:x86_64-linux-android \
            x86:i686-linux-android24:i686-linux-android; do
  IFS=: read -r name target include <<<"$spec"
  mkdir -p "$output/$name"
  ir=$build/$name.ll obfuscated=$build/$name.obfuscated.bc portable=$build/$name.portable.ll object=$build/$name.o
  "$clang" --target="$target" --sysroot="$stock/sysroot" -isystem "$stock/sysroot/usr/include/$include" \
    -O1 -fPIC -fvisibility=hidden -fno-exceptions -fno-rtti -fstack-protector-strong -S -emit-llvm "$source" -o "$ir"
  "$opt" -passes=obfuscation,obf-metrics -obf-seed=41971 "$ir" -o "$obfuscated" > "$build/$name.obfuscation.log"
  # Injected crypto IR carries host CPU attributes; strip them so the Android target decides.
  "$opt" -S "$obfuscated" -o "$portable"
  sed -E -i 's/"(target-cpu|tune-cpu|target-features)"="[^"]*"//g' "$portable"
  "$clang" --target="$target" --sysroot="$stock/sysroot" -O1 -fPIC -fstack-protector-strong -c "$portable" -o "$object"
  "$stock/bin/clang++" --target="$target" --sysroot="$stock/sysroot" -shared -nostdlib++ "$object" \
    -Wl,-z,relro,-z,now,-z,noexecstack,-z,max-page-size=16384 \
    "-Wl,--version-script=$repo/android/sdk-secure/src/main/cpp/exports.map" -o "$output/$name/libdtpsecure.so"
  "$stock/bin/llvm-strip" --strip-unneeded "$output/$name/libdtpsecure.so"
  hash=$(sha256sum "$output/$name/libdtpsecure.so" | cut -d' ' -f1)
  manifest+="{\"abi\":\"$name\",\"sha256\":\"$hash\",\"obfuscator\":\"xollvm-v0.12.0-llvm22\",\"pageAlignment\":16384},"
done
echo "${manifest%,}]" > "$build/manifest.json"
echo "Four obfuscated Android native libraries built."
