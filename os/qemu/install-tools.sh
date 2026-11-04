#!/bin/sh
# Extract pinned Debian amd64 tools into a fresh prefix, without maintainer scripts.
set -eu

if [ "$#" != 1 ]; then
	echo "usage: $0 NEW_PREFIX" >&2
	exit 2
fi
case "$(uname -sm)" in
	"Linux x86_64") ;;
	*) echo "requires Debian-compatible Linux x86_64" >&2; exit 2 ;;
esac
for tool in curl dpkg-deb sha256sum readelf ldd; do
	command -v "$tool" >/dev/null
done
case "$1" in
	/*) prefix=$1 ;;
	*) prefix=$PWD/$1 ;;
esac
# mkdir must fail for an existing file, directory or symlink.
mkdir "$prefix"
prefix=$(cd "$prefix" && pwd -P)
mkdir "$prefix/downloads" "$prefix/root" "$prefix/bin"

# Official Debian binary-package URLs; SHA256 from Debian's Packages index.
# The package filenames pin versions (Debian epochs are omitted in filenames).
# Libraries below fill only the ldd gaps on forge; the host supplies glibc 2.43
# and its other installed runtime libraries. No apt transaction is performed.
base=https://deb.debian.org/debian
while read -r hash path; do
	file=$prefix/downloads/${path##*/}
	curl -fLsS --retry 2 --connect-timeout 10 --max-time 180 \
		"$base/$path" -o "$file"
	printf '%s  %s\n' "$hash" "$file" | sha256sum -c -
	dpkg-deb --extract "$file" "$prefix/root"
done <<'PACKAGES'
576518c3b59477bc4f94982b34e771a6948ab6143a66f98cb7e032839e9db578 pool/main/q/qemu/qemu-system-x86_11.1.3+ds-1_amd64.deb
17575e9e4d9388a235920a82c7065c38a5bc0d98d2e773134fd915c90bc0c847 pool/main/q/qemu/qemu-system-data_11.1.3+ds-1_all.deb
3b957cbc305adaeafec78db2575bd122107647c653ba8eaa4f4348f9153295d0 pool/main/s/seabios/seabios_1.17.0-1_all.deb
c478b439b05f24e87098532daf9c7dff273a5980b64b2b7562e3b33bdfb9e2dc pool/main/b/btrfs-progs/btrfs-progs_7.3-1_amd64.deb
0ecf4b637b1a56e77d7f440bcadd30eb2e8f038e784258730f602201c951156f pool/main/c/cpio/cpio_2.15+dfsg-2.1_amd64.deb
26801f17e6c88e813be104effc0ea3b43d912bd59ca6295fcd1260528ebb4d41 pool/main/b/busybox/busybox-static_1.38.0-3+b1_amd64.deb
6f3e7535b38be149dfae8e079a2c85d07d3f158a3354cf232e98efb8fc925501 pool/main/c/capstone/libcapstone5_5.0.9-1_amd64.deb
3fad48bcc3830a8d236678042e246121cfac26931311874d0a69f862dd82e5d3 pool/main/libs/libslirp/libslirp0_4.9.5-1_amd64.deb
ee0d7248995ff36fc0a31128318c09ebdb86ad2dd84b544816a2af962fd6a1b3 pool/main/v/vdeplug4/libvdeplug2t64_4.0.1-5.1+b2_amd64.deb
7d972d6958e915ce602e4b90debc6b4b91a01dfea1fe003d0b013756e7d9d863 pool/main/p/pmdk/libpmem1_1.13.1-1.1+b1_amd64.deb
dce6da532fc2327810404e72b68883ea077c27c2b11b5d5eac2201ce95017e58 pool/main/d/device-tree-compiler/libfdt1_1.8.1-2_amd64.deb
90612397d7817113c28c56a42e67116bbfb4f3f857ee1520e0509ac9d8616a72 pool/main/liba/libaio/libaio1t64_0.3.113-10_amd64.deb
e1034703cc586b452f4e7ed39fdeba826e338d7796b45dde3965ab47fa76cb1e pool/main/libu/liburing/liburing2_2.15-1_amd64.deb
43a1e7c3b094ac54b19e673a7ddeadd312437df6fdb35ee663c7db95f180a665 pool/main/libe/libexecs/libexecs1_1.4-2+b3_amd64.deb
e36acdb4aeccd0f70d474097e04a1ed920c3c16ab7ae173df2683c2cbeddded4 pool/main/n/ndctl/libdaxctl1_85-1_amd64.deb
d13c8b0b2127be37ebea10b82a6ae4f467adf217e910a19f29c52476f790c6e9 pool/main/n/ndctl/libndctl6_85-1_amd64.deb
PACKAGES

# Debian keeps SeaBIOS in a separate directory. Extraction alone does not add
# its firmware aliases to QEMU's search directory.
for firmware in "$prefix/root/usr/share/seabios/"*.bin; do
	ln -s "../seabios/${firmware##*/}" "$prefix/root/usr/share/qemu/${firmware##*/}"
done

# Keep library and firmware lookup local to each tool invocation.
cat > "$prefix/bin/tool" <<'WRAPPER'
#!/bin/sh
set -eu
prefix=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
export LD_LIBRARY_PATH="$prefix/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
case "${0##*/}" in
	qemu-system-x86_64)
		exec "$prefix/root/usr/bin/qemu-system-x86_64" \
			-L "$prefix/root/usr/share/qemu" "$@" ;;
	mkfs.btrfs)
		exec "$prefix/root/usr/sbin/${0##*/}" "$@" ;;
	btrfs|cpio|busybox)
		exec "$prefix/root/usr/bin/${0##*/}" "$@" ;;
	*) echo "invoke a tool symlink" >&2; exit 2 ;;
esac
WRAPPER
chmod 755 "$prefix/bin/tool"
for tool in qemu-system-x86_64 mkfs.btrfs btrfs cpio busybox; do
	ln -s tool "$prefix/bin/$tool"
done

export LD_LIBRARY_PATH="$prefix/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
for binary in usr/bin/qemu-system-x86_64 usr/sbin/mkfs.btrfs usr/bin/btrfs usr/bin/cpio; do
	ldd "$prefix/root/$binary" > "$prefix/${binary##*/}.ldd"
	if grep -F 'not found' "$prefix/${binary##*/}.ldd"; then
		echo "unresolved runtime libraries; see $prefix/*.ldd" >&2
		exit 1
	fi
done
readelf -lW "$prefix/root/usr/bin/busybox" > "$prefix/busybox.elf"
if grep -E 'INTERP|DYNAMIC' "$prefix/busybox.elf"; then
	echo "BusyBox must be static" >&2
	exit 1
fi
"$prefix/bin/qemu-system-x86_64" --version
"$prefix/bin/cpio" --version
"$prefix/bin/mkfs.btrfs" --version
"$prefix/bin/busybox" | head -n 1
test -r "$prefix/root/usr/share/qemu/bios-256k.bin"
printf 'Tools ready: %s/bin\n' "$prefix"
