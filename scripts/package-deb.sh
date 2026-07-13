#!/usr/bin/env bash
set -euo pipefail

if (($# != 3)); then
  echo "usage: $0 VERSION BINARY OUTPUT_DIRECTORY" >&2
  exit 2
fi

version=$1
binary=$2
output_directory=$3

if [[ ! $version =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "invalid release version: $version" >&2
  exit 2
fi
if [[ ! -f $binary || ! -x $binary ]]; then
  echo "release binary is missing or not executable: $binary" >&2
  exit 2
fi

package_root=$(mktemp -d)
trap 'rm -rf "$package_root"' EXIT
chmod 0755 "$package_root"

install -Dm755 "$binary" "$package_root/usr/bin/notevault"
install -Dm644 packaging/linux/notevault.desktop \
  "$package_root/usr/share/applications/notevault.desktop"
install -Dm644 build/appicon.svg \
  "$package_root/usr/share/icons/hicolor/scalable/apps/notevault.svg"
install -Dm644 build/appicon.png \
  "$package_root/usr/share/icons/hicolor/512x512/apps/notevault.png"
install -Dm644 packaging/debian/copyright \
  "$package_root/usr/share/doc/notevault/copyright"
install -d -m755 "$package_root/DEBIAN"
sed "s/@VERSION@/$version/" packaging/debian/control >"$package_root/DEBIAN/control"
chmod 0644 "$package_root/DEBIAN/control"

mkdir -p "$output_directory"
dpkg-deb --root-owner-group --build "$package_root" \
  "$output_directory/notevault_${version}_amd64.deb"
