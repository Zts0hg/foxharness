#!/bin/sh
set -eu

umask 077

REPOSITORY=Zts0hg/foxharness
BEGIN_MARKER='# >>> foxharness installer >>>'
END_MARKER='# <<< foxharness installer <<<'

TEMP_DIR=
STAGED_BINARY=
PROFILE_STAGE=
PROFILE_SNAPSHOT=
PROFILE_PATH=
PROFILE_STATE=none
PROFILE_CHANGED=0

cleanup() {
  status=$?
  trap - 0 1 2 15

  if [ -n "$PROFILE_STAGE" ]; then
    rm -f "$PROFILE_STAGE" >/dev/null 2>&1 || :
  fi
  if [ -n "$PROFILE_SNAPSHOT" ]; then
    rm -f "$PROFILE_SNAPSHOT" >/dev/null 2>&1 || :
  fi
  if [ -n "$STAGED_BINARY" ]; then
    rm -f "$STAGED_BINARY" >/dev/null 2>&1 || :
  fi
  if [ -n "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR" >/dev/null 2>&1 || :
  fi

  exit "$status"
}

trap cleanup 0
trap 'exit 129' 1
trap 'exit 130' 2
trap 'exit 143' 15

die() {
  printf 'fox installer: %s\n' "$1" >&2
  exit 1
}

usage() {
  cat <<'EOF'
Usage: install.sh [OPTIONS]

Install fox for the current macOS or Linux platform.

Options:
  --version VERSION       Install latest (default) or an exact vMAJOR.MINOR.PATCH.
  --install-dir DIRECTORY Install into DIRECTORY (default: $HOME/.local/bin).
  --no-modify-path        Do not update a shell profile.
  -h, --help              Show this help text.

Environment:
  FOX_VERSION             Same as --version.
  FOX_INSTALL_DIR         Same as --install-dir.
  FOX_NO_MODIFY_PATH      Empty, 0, false, or no means false;
                          1, true, or yes means true (case-insensitive).

Command-line options override environment variables, which override defaults.
When piping the installer, attach environment variables to the sh process:

  curl -fsSL https://github.com/Zts0hg/foxharness/releases/latest/download/install.sh | FOX_VERSION=v0.1.30 sh
  curl -fsSL https://github.com/Zts0hg/foxharness/releases/latest/download/install.sh | FOX_INSTALL_DIR="$HOME/bin" sh

Pass command-line options to a piped script with sh -s --:

  curl -fsSL https://github.com/Zts0hg/foxharness/releases/latest/download/install.sh | sh -s -- --version v0.1.30
EOF
}

usage_error() {
  printf 'fox installer: %s\n\n' "$1" >&2
  usage >&2
  exit 1
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "required command not found: $1"
  fi
}

validate_install_directory() {
  directory=$1
  case "$directory" in
    ''|*:*|*"$carriage_return"*|*"$line_feed"*)
      die 'Invalid installation directory: colons and line breaks are not supported'
      ;;
  esac
}

resolve_physical_directory() {
  unresolved_directory=$1
  if ! physical_output=$(
    CDPATH= cd -P "$unresolved_directory" &&
      pwd -P &&
      printf 'x'
  ); then
    return 1
  fi

  physical_output=${physical_output%x}
  case "$physical_output" in
    *"$line_feed") ;;
    *) return 1 ;;
  esac
  PHYSICAL_DIRECTORY=${physical_output%"$line_feed"}
}

shell_quote() {
  if ! escaped=$(printf '%s' "$1" | LC_ALL=C sed "s/'/'\\\\''/g"); then
    return 1
  fi
  printf "'%s'" "$escaped"
}

path_contains_directory() {
  FOX_PATH_VALUE=${PATH-} FOX_PATH_DIRECTORY=$INSTALL_DIR awk '
    BEGIN {
      count = split(ENVIRON["FOX_PATH_VALUE"], parts, ":")
      for (i = 1; i <= count; i++) {
        if (parts[i] == ENVIRON["FOX_PATH_DIRECTORY"]) {
          exit 0
        }
      }
      exit 1
    }
  '
}

resolve_home() {
  if [ -z "${HOME-}" ]; then
    die 'HOME is required to select a shell profile'
  fi
  if [ ! -d "$HOME" ]; then
    die "HOME is not a directory: $HOME"
  fi
  if ! resolve_physical_directory "$HOME"; then
    die "could not resolve HOME: $HOME"
  fi
  HOME_PHYSICAL=$PHYSICAL_DIRECTORY
}

inspect_profile_markers() {
  marker_file=$1
  if ! PROFILE_STATE=$(
    FOX_BEGIN_MARKER=$BEGIN_MARKER FOX_END_MARKER=$END_MARKER awk '
      BEGIN {
        state = 0
        begin_count = 0
        end_count = 0
        malformed = 0
      }
      $0 == ENVIRON["FOX_BEGIN_MARKER"] {
        begin_count++
        if (state != 0) {
          malformed = 1
        }
        state = 1
        next
      }
      $0 == ENVIRON["FOX_END_MARKER"] {
        end_count++
        if (state != 1) {
          malformed = 1
        }
        state = 2
        next
      }
      END {
        if (begin_count == 0 && end_count == 0) {
          print "none"
        } else if (!malformed && begin_count == 1 && end_count == 1 && state == 2) {
          print "complete"
        } else {
          print "malformed"
        }
      }
    ' "$marker_file"
  ); then
    die "could not inspect shell profile: $PROFILE_PATH"
  fi

  if [ "$PROFILE_STATE" = malformed ]; then
    die "malformed foxharness installer block in $PROFILE_PATH"
  fi
}

inspect_profile() {
  PROFILE_STATE=none

  if [ -L "$PROFILE_PATH" ]; then
    die "shell profile is a symbolic link; rerun with --no-modify-path: $PROFILE_PATH"
  fi
  if [ -e "$PROFILE_PATH" ]; then
    if [ ! -f "$PROFILE_PATH" ]; then
      die "shell profile is not a regular file: $PROFILE_PATH"
    fi
    inspect_profile_markers "$PROFILE_PATH"
  fi
}

select_profile() {
  resolve_home
  shell_name=${SHELL-}
  shell_name=${shell_name##*/}

  case "$PLATFORM_OS:$shell_name" in
    darwin:zsh) profile_name=.zprofile ;;
    darwin:bash) profile_name=.bash_profile ;;
    linux:zsh) profile_name=.zshrc ;;
    linux:bash) profile_name=.bashrc ;;
    *) profile_name=.profile ;;
  esac

  PROFILE_PATH=$HOME_PHYSICAL/$profile_name
  inspect_profile
}

update_profile() {
  PROFILE_EXISTED=0
  inspect_profile
  if [ -f "$PROFILE_PATH" ]; then
    if ! PROFILE_SNAPSHOT=$(mktemp "$HOME_PHYSICAL/.fox-profile-snapshot.XXXXXX"); then
      die "could not snapshot shell profile in $HOME_PHYSICAL"
    fi
    if ! cp -p "$PROFILE_PATH" "$PROFILE_SNAPSHOT"; then
      die "could not snapshot shell profile: $PROFILE_PATH"
    fi
    PROFILE_EXISTED=1
    inspect_profile_markers "$PROFILE_SNAPSHOT"
  fi

  if ! PROFILE_STAGE=$(mktemp "$HOME_PHYSICAL/.fox-profile.XXXXXX"); then
    die "could not create a temporary shell profile in $HOME_PHYSICAL"
  fi

  if [ "$PROFILE_EXISTED" -eq 1 ]; then
    if ! cp -p "$PROFILE_SNAPSHOT" "$PROFILE_STAGE"; then
      die "could not stage shell profile: $PROFILE_PATH"
    fi
  fi

  if [ "$PROFILE_STATE" = complete ]; then
    if ! FOX_BEGIN_MARKER=$BEGIN_MARKER \
      FOX_END_MARKER=$END_MARKER \
      FOX_PATH_LINE=$PATH_LINE \
      awk '
        BEGIN { skipping = 0 }
        $0 == ENVIRON["FOX_BEGIN_MARKER"] {
          print ENVIRON["FOX_BEGIN_MARKER"]
          print ENVIRON["FOX_PATH_LINE"]
          print ENVIRON["FOX_END_MARKER"]
          skipping = 1
          next
        }
        skipping {
          if ($0 == ENVIRON["FOX_END_MARKER"]) {
            skipping = 0
          }
          next
        }
        { print }
      ' "$PROFILE_SNAPSHOT" >"$PROFILE_STAGE"; then
      die "could not update shell profile: $PROFILE_PATH"
    fi
  else
    if [ "$PROFILE_EXISTED" -eq 1 ]; then
      if ! cat "$PROFILE_SNAPSHOT" >"$PROFILE_STAGE"; then
        die "could not copy shell profile: $PROFILE_PATH"
      fi
      if [ -s "$PROFILE_SNAPSHOT" ]; then
        printf '\n' >>"$PROFILE_STAGE" || die "could not update shell profile: $PROFILE_PATH"
      fi
    else
      : >"$PROFILE_STAGE" || die "could not stage shell profile: $PROFILE_PATH"
    fi
    if ! printf '%s\n%s\n%s\n' "$BEGIN_MARKER" "$PATH_LINE" "$END_MARKER" >>"$PROFILE_STAGE"; then
      die "could not update shell profile: $PROFILE_PATH"
    fi
  fi

  if [ "$PROFILE_EXISTED" -eq 1 ]; then
    if [ -L "$PROFILE_PATH" ] || [ ! -f "$PROFILE_PATH" ] || ! cmp -s "$PROFILE_SNAPSHOT" "$PROFILE_PATH"; then
      die "shell profile changed during installation: $PROFILE_PATH"
    fi
  elif [ -e "$PROFILE_PATH" ] || [ -L "$PROFILE_PATH" ]; then
    die "shell profile changed during installation: $PROFILE_PATH"
  fi

  if ! mv -f "$PROFILE_STAGE" "$PROFILE_PATH"; then
    die "could not replace shell profile: $PROFILE_PATH"
  fi
  PROFILE_STAGE=
  PROFILE_CHANGED=1
}

download_file() {
  url=$1
  output=$2

  case "$DOWNLOADER" in
    curl)
      if ! curl -fsSL "$url" -o "$output"; then
        die "download failed: $url"
      fi
      ;;
    wget)
      if ! wget -q "$url" -O "$output"; then
        die "download failed: $url"
      fi
      ;;
  esac
}

calculate_sha256() {
  file=$1
  case "$HASH_TOOL" in
    sha256sum)
      digest_output=$(sha256sum "$file") || return 1
      printf '%s\n' "$digest_output" | awk 'NR == 1 { print tolower($1) }'
      ;;
    shasum)
      digest_output=$(shasum -a 256 "$file") || return 1
      printf '%s\n' "$digest_output" | awk 'NR == 1 { print tolower($1) }'
      ;;
    openssl)
      digest_output=$(openssl dgst -sha256 "$file") || return 1
      printf '%s\n' "$digest_output" | awk 'NR == 1 { print tolower($NF) }'
      ;;
  esac
}

if [ "${FOX_VERSION+x}" = x ]; then
  VERSION=$FOX_VERSION
else
  VERSION=latest
fi
if [ "${FOX_INSTALL_DIR+x}" = x ]; then
  INSTALL_DIR=$FOX_INSTALL_DIR
  INSTALL_DIR_ENV_SET=1
else
  INSTALL_DIR=
  INSTALL_DIR_ENV_SET=0
fi
INSTALL_DIR_CLI_SET=0
NO_MODIFY_PATH_CLI_SET=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || usage_error '--version requires a value'
      VERSION=$2
      shift 2
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || usage_error '--install-dir requires a value'
      INSTALL_DIR=$2
      INSTALL_DIR_CLI_SET=1
      shift 2
      ;;
    --no-modify-path)
      NO_MODIFY_PATH_CLI_SET=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage_error "Unknown argument: $1"
      ;;
  esac
done

for required in awk cat chmod cmp cp grep mkdir mktemp mv pwd rm sed tar tr uname
do
  require_command "$required"
done

carriage_return=$(printf '\r')
line_feed='
'
case "$VERSION" in
  *"$carriage_return"*|*"$line_feed"*)
    die "Invalid fox version: line breaks are not supported"
    ;;
  latest) ;;
  *)
    if ! printf '%s\n' "$VERSION" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
      die "Invalid fox version: $VERSION (expected latest or vMAJOR.MINOR.PATCH)"
    fi
    ;;
esac

if [ "$NO_MODIFY_PATH_CLI_SET" -eq 1 ]; then
  NO_MODIFY_PATH=1
else
  no_modify_value=${FOX_NO_MODIFY_PATH-}
  case "$no_modify_value" in
    *"$carriage_return"*|*"$line_feed"*) die "Invalid FOX_NO_MODIFY_PATH value: line breaks are not supported" ;;
  esac
  no_modify_value=$(printf '%s' "$no_modify_value" | LC_ALL=C tr '[:upper:]' '[:lower:]')
  case "$no_modify_value" in
    ''|0|false|no) NO_MODIFY_PATH=0 ;;
    1|true|yes) NO_MODIFY_PATH=1 ;;
    *) die "Invalid FOX_NO_MODIFY_PATH value: ${FOX_NO_MODIFY_PATH-}" ;;
  esac
fi

if [ -z "$INSTALL_DIR" ]; then
  if [ "$INSTALL_DIR_CLI_SET" -eq 1 ] || [ "$INSTALL_DIR_ENV_SET" -eq 1 ]; then
    die 'Invalid installation directory: the value must not be empty'
  fi
  if [ -z "${HOME-}" ]; then
    die 'HOME is required for the default installation directory'
  fi
  INSTALL_DIR=$HOME/.local/bin
fi

validate_install_directory "$INSTALL_DIR"

detected_os=$(uname -s) || die 'could not detect the operating system'
case "$detected_os" in
  Darwin) PLATFORM_OS=darwin ;;
  Linux) PLATFORM_OS=linux ;;
  *) die "Unsupported operating system: $detected_os" ;;
esac

detected_arch=$(uname -m) || die 'could not detect the architecture'
if [ "$PLATFORM_OS" = darwin ] && [ "$detected_arch" = x86_64 ]; then
  if command -v sysctl >/dev/null 2>&1; then
    translated=$(sysctl -n sysctl.proc_translated 2>/dev/null || printf '0')
    if [ "$translated" = 1 ]; then
      detected_arch=arm64
    fi
  fi
fi

case "$detected_arch" in
  x86_64|amd64) PLATFORM_ARCH=amd64 ;;
  arm64|aarch64) PLATFORM_ARCH=arm64 ;;
  *) die "Unsupported architecture: $detected_arch" ;;
esac

case "$INSTALL_DIR" in
  /*) requested_install_dir=$INSTALL_DIR ;;
  *)
    if ! resolve_physical_directory .; then
      die 'could not resolve the current directory'
    fi
    current_directory=$PHYSICAL_DIRECTORY
    requested_install_dir=$current_directory/$INSTALL_DIR
    ;;
esac

if ! mkdir -p "$requested_install_dir"; then
  die "could not create installation directory: $requested_install_dir"
fi
if [ ! -d "$requested_install_dir" ]; then
  die "installation path is not a directory: $requested_install_dir"
fi
if ! resolve_physical_directory "$requested_install_dir"; then
  die "could not resolve installation directory: $requested_install_dir"
fi
INSTALL_DIR=$PHYSICAL_DIRECTORY
validate_install_directory "$INSTALL_DIR"
if [ ! -w "$INSTALL_DIR" ]; then
  die "installation directory is not writable: $INSTALL_DIR"
fi

if ! PATH_WORD=$(shell_quote "$INSTALL_DIR"); then
  die 'could not serialize the installation directory for PATH'
fi
PATH_LINE="export PATH=$PATH_WORD:\"\$PATH\""

PATH_MISSING=1
if path_contains_directory; then
  PATH_MISSING=0
elif [ "$NO_MODIFY_PATH" -eq 0 ]; then
  select_profile
fi

if command -v curl >/dev/null 2>&1; then
  DOWNLOADER=curl
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER=wget
else
  die 'curl or wget is required to download fox'
fi

if command -v sha256sum >/dev/null 2>&1; then
  HASH_TOOL=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  HASH_TOOL=shasum
elif command -v openssl >/dev/null 2>&1; then
  HASH_TOOL=openssl
else
  die 'sha256sum, shasum, or openssl is required to verify fox'
fi

ASSET=fox_${PLATFORM_OS}_${PLATFORM_ARCH}.tar.gz
if [ "$VERSION" = latest ]; then
  BASE_URL=https://github.com/$REPOSITORY/releases/latest/download
else
  BASE_URL=https://github.com/$REPOSITORY/releases/download/$VERSION
fi
ARCHIVE_URL=$BASE_URL/$ASSET
CHECKSUM_URL=$ARCHIVE_URL.sha256

tmp_base=${TMPDIR:-/tmp}
if ! TEMP_DIR=$(mktemp -d "$tmp_base/fox-install.XXXXXX"); then
  die "could not create a private temporary directory in $tmp_base"
fi
ARCHIVE_PATH=$TEMP_DIR/$ASSET
CHECKSUM_PATH=$ARCHIVE_PATH.sha256

printf 'Downloading fox (%s/%s, %s)...\n' "$PLATFORM_OS" "$PLATFORM_ARCH" "$VERSION"
download_file "$ARCHIVE_URL" "$ARCHIVE_PATH"
download_file "$CHECKSUM_URL" "$CHECKSUM_PATH"

if ! EXPECTED_DIGEST=$(
  FOX_CHECKSUM_ASSET=$ASSET awk '
    NR == 1 {
      digest = $1
      filename = $2
      if (substr(filename, 1, 1) == "*") {
        filename = substr(filename, 2)
      }
      if (NF != 2 || length(digest) != 64 || digest ~ /[^0-9A-Fa-f]/ || filename != ENVIRON["FOX_CHECKSUM_ASSET"]) {
        malformed = 1
      }
    }
    NR > 1 { malformed = 1 }
    END {
      if (NR != 1 || malformed) {
        exit 1
      }
      print tolower(digest)
    }
  ' "$CHECKSUM_PATH"
); then
  die "invalid checksum file for $ASSET"
fi

if ! ACTUAL_DIGEST=$(calculate_sha256 "$ARCHIVE_PATH"); then
  die "could not calculate the SHA-256 checksum for $ASSET"
fi
if [ "${#ACTUAL_DIGEST}" -ne 64 ] || ! printf '%s\n' "$ACTUAL_DIGEST" | LC_ALL=C grep -Eq '^[0-9a-f]{64}$'; then
  die "invalid SHA-256 output while verifying $ASSET"
fi
if [ "$EXPECTED_DIGEST" != "$ACTUAL_DIGEST" ]; then
  die "checksum mismatch for $ASSET"
fi

NAMES_PATH=$TEMP_DIR/archive-names
VERBOSE_PATH=$TEMP_DIR/archive-verbose
if ! tar -tzf "$ARCHIVE_PATH" >"$NAMES_PATH"; then
  die "unsafe archive: could not list $ASSET"
fi
if ! ARCHIVE_ENTRY=$(awk 'NR == 1 { entry = $0 } END { if (NR != 1) exit 1; print entry }' "$NAMES_PATH"); then
  die 'unsafe archive: expected exactly one entry'
fi
case "$ARCHIVE_ENTRY" in
  fox|./fox) ;;
  *) die "unsafe archive: unexpected entry $ARCHIVE_ENTRY" ;;
esac

if ! tar -tvzf "$ARCHIVE_PATH" >"$VERBOSE_PATH"; then
  die "unsafe archive: could not inspect $ASSET"
fi
if ! ARCHIVE_TYPE=$(awk 'NR == 1 { type = substr($0, 1, 1) } END { if (NR != 1) exit 1; print type }' "$VERBOSE_PATH"); then
  die 'unsafe archive: expected exactly one typed entry'
fi
if [ "$ARCHIVE_TYPE" != '-' ]; then
  die 'unsafe archive: fox is not a regular file'
fi

EXTRACT_DIR=$TEMP_DIR/extracted
if ! mkdir "$EXTRACT_DIR"; then
  die 'could not create the private extraction directory'
fi
if ! tar -xzf "$ARCHIVE_PATH" -C "$EXTRACT_DIR" "$ARCHIVE_ENTRY"; then
  die "could not extract verified archive: $ASSET"
fi
CANDIDATE=$EXTRACT_DIR/fox
if [ ! -f "$CANDIDATE" ] || [ -L "$CANDIDATE" ]; then
  die 'unsafe archive: extracted fox is not a regular file'
fi

DESTINATION=$INSTALL_DIR/fox
if [ -d "$DESTINATION" ]; then
  die "destination is a directory: $DESTINATION"
fi
if ! STAGED_BINARY=$(mktemp "$INSTALL_DIR/.fox.install.XXXXXX"); then
  die "could not create a staging file in $INSTALL_DIR"
fi
if ! cp "$CANDIDATE" "$STAGED_BINARY"; then
  die "could not stage fox in $INSTALL_DIR"
fi
if ! chmod 0755 "$STAGED_BINARY"; then
  die 'could not make the staged fox executable'
fi
if [ ! -f "$STAGED_BINARY" ] || [ -L "$STAGED_BINARY" ] || [ ! -x "$STAGED_BINARY" ]; then
  die 'staged fox is not an executable regular file'
fi

if [ "$PATH_MISSING" -eq 1 ] && [ "$NO_MODIFY_PATH" -eq 0 ]; then
  update_profile
fi

if ! mv -f "$STAGED_BINARY" "$DESTINATION"; then
  die "could not atomically install fox at $DESTINATION"
fi
STAGED_BINARY=

printf 'Installed fox to %s\n' "$DESTINATION"
if [ "$PATH_MISSING" -eq 1 ]; then
  if [ "$PROFILE_CHANGED" -eq 1 ]; then
    printf 'Updated %s so future shells can find fox.\n' "$PROFILE_PATH"
  else
    printf 'Your shell PATH was not modified.\n'
  fi
  printf 'To use fox in the current shell, run:\n%s\n' "$PATH_LINE"
fi
