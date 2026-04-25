FROM --platform=linux/amd64 debian:stable-slim

ARG ZIG_VERSION=0.16.0

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl xz-utils cmake ninja-build pkg-config \
      file \
    && rm -rf /var/lib/apt/lists/*

RUN dpkg --add-architecture arm64 \
    && apt-get update && apt-get install -y --no-install-recommends \
         libx11-dev libx11-dev:arm64 \
    && rm -rf /var/lib/apt/lists/*

RUN curl -sL https://ziglang.org/download/${ZIG_VERSION}/zig-x86_64-linux-${ZIG_VERSION}.tar.xz \
    | tar -xJ -C /opt \
    && ln -s /opt/zig-x86_64-linux-${ZIG_VERSION}/zig /usr/local/bin/zig

COPY cross/toolchains/zig-ar cross/toolchains/zig-ranlib /usr/local/bin/
RUN chmod +x /usr/local/bin/zig-ar /usr/local/bin/zig-ranlib

# appimagetool for AppImage packaging at release step 6. The AppImage ships
# with a FUSE-based runtime, but FUSE is not available inside this container;
# invocations set APPIMAGE_EXTRACT_AND_RUN=1 to sidestep that dep. The x86_64
# tool produces both amd64 and arm64 output when called with ARCH=<target>.
# Pinned to the aperturerobotics/appimagetool fork so the release is stable
# and the sha256 stays verifiable (upstream continuous moves under us).
#
# sha256 is verified BEFORE we patch the AppImage type-2 magic bytes. The
# patch zeros three bytes at ELF offset 8 ("AI\x02") that the kernel loader
# ignores but qemu-user's stricter ELF parser rejects as ENOEXEC; without
# it, running this container on a non-amd64 host (Docker Desktop under
# qemu emulation on darwin-arm64) fails with "exec format error". See
# https://github.com/AppImage/AppImageKit/issues/828.
#
# The zeroing uses `dd if=/dev/zero` rather than `printf '\x00'` because
# Debian's /bin/sh (dash) does not interpret `\x` hex escapes; dash would
# emit the literal 9-byte string "\x00\x00\x00" and the magic would stay
# non-zero, still tripping qemu-user on the next RUN.
ARG APPIMAGETOOL_VERSION=20260418
ARG APPIMAGETOOL_X86_64_SHA256=a15012a5f782b93d0e49df93f2ed584e19bb0e0e113d92df060f039c92cb9d94
RUN curl -sL \
      "https://github.com/aperturerobotics/appimagetool/releases/download/${APPIMAGETOOL_VERSION}/appimagetool-x86_64.AppImage" \
      -o /usr/local/bin/appimagetool \
    && echo "${APPIMAGETOOL_X86_64_SHA256}  /usr/local/bin/appimagetool" | sha256sum -c - \
    && dd if=/dev/zero of=/usr/local/bin/appimagetool bs=1 seek=8 count=3 conv=notrunc status=none \
    && chmod +x /usr/local/bin/appimagetool \
    && APPIMAGE_EXTRACT_AND_RUN=1 /usr/local/bin/appimagetool --version

# BLOCKED 2026-04-20: Azure identity validation is approved, but the active
# release path still lacks the Windows-hosted signing hop required for MSIX.
# `signtool.exe` (the binary `az sign` shells out to) is Windows-only, so the
# packaging half can run in this Linux builder but the sign step cannot.
#
# When Windows-hosted signing wiring lands: uncomment, rebuild the image,
# re-enable the `scripts/build-msix.sh` invocation in release.go step 6, and
# restore the makemsix probe in scripts/ensure-builder-image.sh.
#
# Historical context (kept for re-enable): pinned to tag v1.7 (Microsoft's
# last tagged release). msix-packaging's upstream toolchain forces C++14, but
# Debian trixie's libicu-dev (ICU 76) headers require C++17. Patching the
# MSIX_INTERFACE macro in AppxPackaging.hpp wraps templates inside extern
# "C++" so GCC + C++17 compiles clean. -DMSIX_PACK=on is the only switch
# needed; samples + tests are disabled to keep the build short.
#
# ARG MSIX_PACKAGING_VERSION=v1.7
# RUN apt-get update && apt-get install -y --no-install-recommends \
#       git g++ libssl-dev libicu-dev \
#     && rm -rf /var/lib/apt/lists/*
# RUN git clone --depth 1 --branch "${MSIX_PACKAGING_VERSION}" \
#         https://github.com/microsoft/msix-packaging.git /tmp/msix-packaging \
#     && find /tmp/msix-packaging -name CMakeLists.txt \
#         -exec sed -i 's/CMAKE_CXX_STANDARD 14/CMAKE_CXX_STANDARD 17/g' {} + \
#     && perl -i -0pe 's/#define MSIX_INTERFACE\(name,l,w1,w2,b1,b2,b3,b4,b5,b6,b7,b8\).*?    \};/#define MSIX_INTERFACE(name,l,w1,w2,b1,b2,b3,b4,b5,b6,b7,b8) extern "C++" { static constexpr const IID IID_##name = {l,w1,w2,{b1,b2,b3,b4,b5,b6,b7,b8}}; template<> struct UuidOfImpl<name> { static constexpr const IID\&amp; iid = IID_##name; }; }/s' \
#         /tmp/msix-packaging/src/inc/AppxPackaging.hpp \
#     && mkdir -p /tmp/msix-packaging/.vs \
#     && cd /tmp/msix-packaging/.vs \
#     && cmake -G Ninja \
#         -DCMAKE_BUILD_TYPE=MinSizeRel \
#         -DCMAKE_C_COMPILER=gcc \
#         -DCMAKE_CXX_COMPILER=g++ \
#         -DMSIX_PACK=on \
#         -DUSE_VALIDATION_PARSER=on \
#         -DMSIX_SAMPLES=off \
#         -DMSIX_TESTS=off \
#         -DLINUX=on \
#         .. \
#     && ninja makemsix \
#     && install -m 0755 bin/makemsix /usr/local/bin/makemsix \
#     && cd / && rm -rf /tmp/msix-packaging \
#     && /usr/local/bin/makemsix --help >/dev/null

WORKDIR /src
