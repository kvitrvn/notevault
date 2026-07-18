#!/usr/bin/env bash
set -euo pipefail

if (($# != 1)); then
  echo "usage: $0 PACKAGE" >&2
  exit 2
fi

package=$1
if [[ ! -f $package ]]; then
  echo "package not found: $package" >&2
  exit 2
fi

case $package in
  *.deb)
    contents=$(dpkg-deb --fsys-tarfile "$package" | tar -tf -)
    control=$(dpkg-deb --ctrl-tarfile "$package" | tar -tf -)
    details=$(dpkg-deb --contents "$package")
    for required in \
      ./usr/bin/notevault \
      ./usr/share/applications/notevault.desktop \
      ./usr/share/icons/hicolor/scalable/apps/notevault.svg \
      ./usr/share/icons/hicolor/512x512/apps/notevault.png \
      ./usr/share/doc/notevault/copyright \
      ./usr/share/doc/notevault/THIRD_PARTY_NOTICES.md; do
      grep -Fxq "$required" <<<"$contents" || {
        echo "missing package path: $required" >&2
        exit 1
      }
    done
    if grep -Eq '^\./(preinst|postinst|prerm|postrm|config|templates)$' <<<"$control"; then
      echo "Debian maintainer scripts are forbidden" >&2
      exit 1
    fi
    [[ ${details%%$'\n'*} == "drwxr-xr-x"*" ./" ]] || {
      echo "Debian package root must use mode 0755" >&2
      exit 1
    }
    grep -Eq '^-rwxr-xr-x .* \./usr/bin/notevault$' <<<"$details" || {
      echo "Debian binary must use mode 0755" >&2
      exit 1
    }
    grep -Eq '^-rw-r--r-- .* \./usr/share/applications/notevault.desktop$' \
      <<<"$details" || {
      echo "Debian desktop entry must use mode 0644" >&2
      exit 1
    }
    ;;
  *.pkg.tar.zst)
    if command -v bsdtar >/dev/null 2>&1; then
      contents=$(bsdtar -tf "$package")
      details=$(bsdtar -tvf "$package")
    else
      contents=$(tar --zstd -tf "$package")
      details=$(tar --zstd -tvf "$package")
    fi
    for required in \
      usr/bin/notevault \
      usr/share/applications/notevault.desktop \
      usr/share/icons/hicolor/scalable/apps/notevault.svg \
      usr/share/icons/hicolor/512x512/apps/notevault.png \
      usr/share/licenses/notevault/LICENSE \
      usr/share/licenses/notevault/THIRD_PARTY_NOTICES.md; do
      grep -Fxq "$required" <<<"$contents" || {
        echo "missing package path: $required" >&2
        exit 1
      }
    done
    if grep -Fxq .INSTALL <<<"$contents"; then
      echo "Arch install scripts are forbidden" >&2
      exit 1
    fi
    grep -Eq '^-rwxr-xr-x .* usr/bin/notevault$' <<<"$details" || {
      echo "Arch binary must use mode 0755" >&2
      exit 1
    }
    grep -Eq '^-rw-r--r-- .* usr/share/applications/notevault.desktop$' \
      <<<"$details" || {
      echo "Arch desktop entry must use mode 0644" >&2
      exit 1
    }
    ;;
  *)
    echo "unsupported package format: $package" >&2
    exit 2
    ;;
esac

if grep -Eiq '(^|/)\.config/hypr(/|$)' <<<"$contents"; then
  echo "package must not contain Hyprland user configuration" >&2
  exit 1
fi
