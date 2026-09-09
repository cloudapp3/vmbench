#!/usr/bin/env bash
# Build pinned static Linux binaries of the external benchmark tools and emit
# the SHA-256 pins for toolbin/toolbin.go.
#
# Usage: scripts/build-tools.sh [output-dir]
#
# Requires docker (arm64 builds run under qemu binfmt). Output:
#   <out>/vmbench-tools-fio-linux-{amd64,arm64}
#   <out>/vmbench-tools-sysbench-linux-{amd64,arm64}
#   <out>/sha256sums.txt
#
# Upstream sources are fetched inside the container from the pinned tags
# below; see docs/THIRD-PARTY.md for the license/source offer.
set -euo pipefail

FIO_VERSION="3.39"
SYSBENCH_VERSION="1.0.20"
ALPINE_VERSION="3.20"
OUT_DIR="${1:-dist-tools}"

mkdir -p "$OUT_DIR"

build_fio() {
    local arch="$1"
    docker run --rm --platform "linux/$arch" \
        -e FIO_VERSION="$FIO_VERSION" -e OUT_ARCH="$arch" \
        -v "$PWD/$OUT_DIR":/out \
        "alpine:$ALPINE_VERSION" sh -euxc '
        # zlib is deliberately absent: alpine 3.20 zlib-dev ships no libz.a,
        # so the optional fio zlib feature auto-disables and the static link holds.
        apk add --no-cache gcc make musl-dev linux-headers libaio-dev curl tar file
        curl -fsSLO "https://github.com/axboe/fio/archive/refs/tags/fio-${FIO_VERSION}.tar.gz"
        # GitHub archives extract to <repo>-<tag>/; strip into a fixed dir.
        mkdir src
        tar xf "fio-${FIO_VERSION}.tar.gz" -C src --strip-components=1
        cd src
        ./configure --disable-native
        # fio has no configure --static; static linking goes through LDFLAGS.
        make -j"$(nproc)" fio LDFLAGS="-static"
    file fio
    file fio | grep -q "statically linked" || { echo "fio is not static" >&2; exit 1; }
        ./fio --version
        cp fio "/out/vmbench-tools-fio-linux-${OUT_ARCH}"
'
}

build_sysbench() {
    local arch="$1"
    docker run --rm --platform "linux/$arch" \
        -e SYSBENCH_VERSION="$SYSBENCH_VERSION" -e OUT_ARCH="$arch" \
        -v "$PWD/$OUT_DIR":/out \
        "alpine:$ALPINE_VERSION" sh -euxc '
        # sysbench autogen.sh requires a bash interpreter.
        apk add --no-cache gcc make musl-dev autoconf automake libtool pkgconfig git bash file
        # The GitHub release tarball omits submodules (luajit, concurrency_kit);
        # clone recursively at the pinned tag instead.
        git clone --depth 1 --branch "$SYSBENCH_VERSION" --recursive \
            https://github.com/akopytov/sysbench.git
        cd sysbench
        ./autogen.sh
        ./configure --without-mysql
        # Build everything first with -k: CK libck.a is produced before its
        # libck.so link fails, and we only need the archives for the final
        # static link of sysbench itself.
        make -k -j"$(nproc)" || true
        rm -f src/sysbench
        # libtool swallows plain -static; -all-static makes the final link
        # resolve only against the .a archives.
        make -C src LDFLAGS="-all-static"
    file src/sysbench
    file src/sysbench | grep -q "statically linked" || { echo "sysbench is not static" >&2; exit 1; }
        ./src/sysbench --version
        cp src/sysbench "/out/vmbench-tools-sysbench-linux-${OUT_ARCH}"
'
}

for arch in amd64 arm64; do
    echo "==> fio $FIO_VERSION linux/$arch"
    build_fio "$arch"
    echo "==> sysbench $SYSBENCH_VERSION linux/$arch"
    build_sysbench "$arch"
done

cd "$OUT_DIR"
sha256sum vmbench-tools-*-linux-* | tee sha256sums.txt
echo ""
echo "Next: upload to the 'tools' release and paste the hashes into toolbin/toolbin.go:"
echo "  gh release upload tools --clobber vmbench-tools-*-linux-* sha256sums.txt"
