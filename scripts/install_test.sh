#!/bin/sh
set -eu

LC_ALL=C
LANG=C
export LC_ALL LANG
TEST_LINE_FEED='
'

SCRIPT_DIR=$(CDPATH= cd "$(dirname "$0")" && pwd)
INSTALLER=$SCRIPT_DIR/install.sh
INSTALL_SHELL=${INSTALL_TEST_SHELL-/bin/sh}
ENV_CMD=$(command -v env)
SUITE_ROOT=$(mktemp -d "${TMPDIR-/tmp}/fox-install-tests.XXXXXX")
PASS_COUNT=0
FAIL_COUNT=0
TOTAL_COUNT=0

cleanup_suite() {
  rm -rf "$SUITE_ROOT"
}

trap cleanup_suite 0
trap 'exit 129' 1
trap 'exit 130' 2
trap 'exit 143' 15

fail() {
  printf '    %s\n' "$1" >&2
  return 1
}

assert_eq() {
  expected=$1
  actual=$2
  message=$3
  if [ "$expected" != "$actual" ]; then
    fail "$message: expected [$expected], got [$actual]"
  fi
}

assert_status() {
  expected=$1
  if [ "$RUN_STATUS" -ne "$expected" ]; then
    fail "expected exit status $expected, got $RUN_STATUS; stderr: $(sed -n '1,3p' "$RUN_ERR")"
  fi
}

assert_nonzero() {
  if [ "$RUN_STATUS" -eq 0 ]; then
    fail "expected a non-zero exit status"
  fi
}

assert_contains() {
  file=$1
  text=$2
  message=$3
  if ! grep -F "$text" "$file" >/dev/null 2>&1; then
    fail "$message: missing [$text] in $(sed -n '1,5p' "$file")"
  fi
}

assert_not_contains() {
  file=$1
  text=$2
  message=$3
  if grep -F "$text" "$file" >/dev/null 2>&1; then
    fail "$message: unexpected [$text]"
  fi
}

assert_exists() {
  path=$1
  if [ ! -e "$path" ]; then
    fail "expected path to exist: $path"
  fi
}

assert_not_exists() {
  path=$1
  if [ -e "$path" ]; then
    fail "expected path not to exist: $path"
  fi
}

assert_executable() {
  path=$1
  if [ ! -f "$path" ] || [ ! -x "$path" ] || [ -L "$path" ]; then
    fail "expected an executable regular file: $path"
  fi
}

assert_symlink() {
  path=$1
  if [ ! -L "$path" ]; then
    fail "expected a symbolic link: $path"
  fi
}

assert_directory_empty() {
  path=$1
  if find "$path" -mindepth 1 -print | grep . >/dev/null 2>&1; then
    fail "expected directory to be empty: $path"
  fi
}

assert_content() {
  path=$1
  expected=$2
  actual=$(sed -n '1p' "$path" 2>/dev/null || true)
  assert_eq "$expected" "$actual" "unexpected file content for $path"
}

assert_no_download() {
  if [ -s "$DOWNLOAD_LOG" ]; then
    fail "expected no download, got: $(sed -n '1,5p' "$DOWNLOAD_LOG")"
  fi
}

file_sha256() {
  path=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{ print tolower($1) }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{ print tolower($1) }'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$path" | sed 's/^.*= //' | tr 'A-F' 'a-f'
  else
    fail "test host needs sha256sum, shasum, or openssl"
  fi
}

write_checksum() {
  archive=$1
  digest=$(file_sha256 "$FIXTURE_DIR/$archive")
  printf '%s  %s\n' "$digest" "$archive" >"$FIXTURE_DIR/$archive.sha256"
}

link_tool() {
  tool=$1
  tool_path=$(command -v "$tool" 2>/dev/null || true)
  if [ -n "$tool_path" ]; then
    case "$tool" in
      cmp)
        printf '#!/bin/sh\nif [ -n "${MUTATE_PROFILE_DURING_CMP-}" ]; then printf "concurrent-profile-change\\n" >>"$MUTATE_PROFILE_DURING_CMP"; fi\nexec "%s" "$@"\n' "$tool_path" >"$FAKEBIN/$tool"
        chmod +x "$FAKEBIN/$tool"
        ;;
      shasum)
        printf '#!/bin/sh\nexec "%s" "$@"\n' "$tool_path" >"$FAKEBIN/$tool"
        chmod +x "$FAKEBIN/$tool"
        ;;
      *)
        ln -s "$tool_path" "$FAKEBIN/$tool"
        ;;
    esac
  fi
}

create_base_tools() {
  for tool in awk basename cat chmod cmp cp cut dirname find grep gzip head mkdir mktemp mv openssl pwd rm sed shasum sha256sum sort tar tr wc
  do
    link_tool "$tool"
  done
}

create_fake_platform_tools() {
  cat >"$FAKEBIN/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) printf '%s\n' "$FAKE_OS" ;;
  -m) printf '%s\n' "$FAKE_ARCH" ;;
  *) exit 2 ;;
esac
EOF
  chmod +x "$FAKEBIN/uname"

  cat >"$FAKEBIN/sysctl" <<'EOF'
#!/bin/sh
if [ "$#" -eq 2 ] && [ "$1" = "-n" ] && [ "$2" = "sysctl.proc_translated" ]; then
  printf '%s\n' "$FAKE_ROSETTA"
  exit 0
fi
exit 1
EOF
  chmod +x "$FAKEBIN/sysctl"
}

create_fake_curl() {
  cat >"$FAKEBIN/curl" <<'EOF'
#!/bin/sh
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      shift
      [ "$#" -gt 0 ] || exit 2
      output=$1
      ;;
    -*)
      ;;
    *)
      url=$1
      ;;
  esac
  shift
done
[ -n "$output" ] && [ -n "$url" ] || exit 2
printf 'curl %s\n' "$url" >>"$DOWNLOAD_LOG"
asset=$(printf '%s\n' "$url" | sed 's#.*/##')
cp "$FIXTURE_DIR/$asset" "$output"
case "$url" in
  *.sha256)
    if [ -n "${MUTATE_PROFILE_ON_CHECKSUM-}" ]; then
      printf '# >>> foxharness installer >>>\n' >"$MUTATE_PROFILE_ON_CHECKSUM"
    fi
    ;;
esac
EOF
  chmod +x "$FAKEBIN/curl"
}

create_fake_wget() {
  cat >"$FAKEBIN/wget" <<'EOF'
#!/bin/sh
output=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -O)
      shift
      [ "$#" -gt 0 ] || exit 2
      output=$1
      ;;
    -*)
      ;;
    *)
      url=$1
      ;;
  esac
  shift
done
[ -n "$output" ] && [ -n "$url" ] || exit 2
printf 'wget %s\n' "$url" >>"$DOWNLOAD_LOG"
asset=$(printf '%s\n' "$url" | sed 's#.*/##')
cp "$FIXTURE_DIR/$asset" "$output"
EOF
  chmod +x "$FAKEBIN/wget"
}

create_valid_fixtures() {
  payload=$CASE_ROOT/payload
  mkdir -p "$payload"
  printf 'fox-test-binary\n' >"$payload/fox"
  for target in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64
  do
    archive=fox_$target.tar.gz
    tar -C "$payload" -czf "$FIXTURE_DIR/$archive" fox
    write_checksum "$archive"
  done
}

setup_case() {
  downloader=$1
  CASE_ROOT=$(mktemp -d "$SUITE_ROOT/case.XXXXXX")
  HOME_DIR=$CASE_ROOT/home
  TMP_DIR=$CASE_ROOT/tmp
  TEST_CWD=$CASE_ROOT/work
  FIXTURE_DIR=$CASE_ROOT/fixtures
  FAKEBIN=$CASE_ROOT/fakebin
  DOWNLOAD_LOG=$CASE_ROOT/download.log
  RUN_OUT=$CASE_ROOT/stdout
  RUN_ERR=$CASE_ROOT/stderr
  mkdir -p "$HOME_DIR" "$TMP_DIR" "$TEST_CWD" "$FIXTURE_DIR" "$FAKEBIN"
  : >"$DOWNLOAD_LOG"

  create_base_tools
  create_fake_platform_tools
  case "$downloader" in
    curl) create_fake_curl ;;
    wget) create_fake_wget ;;
    both) create_fake_curl; create_fake_wget ;;
    none) ;;
    *) fail "unknown test downloader: $downloader" ;;
  esac
  cp "$BASE_FIXTURE_DIR/"* "$FIXTURE_DIR/"

  FAKE_OS=Linux
  FAKE_ARCH=x86_64
  FAKE_ROSETTA=0
  CASE_SHELL=/bin/bash
  PROCESS_PATH=$FAKEBIN
  ENV_VERSION=
  ENV_VERSION_SET=0
  ENV_INSTALL_DIR=
  ENV_INSTALL_DIR_SET=0
  ENV_NO_MODIFY_PATH=
  ENV_NO_MODIFY_PATH_SET=0
  MUTATE_PROFILE_ON_CHECKSUM=
  MUTATE_PROFILE_DURING_CMP=
}

run_installer() {
  version_set=$ENV_VERSION_SET
  install_dir_set=$ENV_INSTALL_DIR_SET
  no_modify_path_set=$ENV_NO_MODIFY_PATH_SET
  [ -n "$ENV_VERSION" ] && version_set=1
  [ -n "$ENV_INSTALL_DIR" ] && install_dir_set=1
  [ -n "$ENV_NO_MODIFY_PATH" ] && no_modify_path_set=1

  set +e
  (
    cd "$TEST_CWD"
    "$ENV_CMD" -i \
      HOME="$HOME_DIR" \
      PATH="$PROCESS_PATH" \
      SHELL="$CASE_SHELL" \
      TMPDIR="$TMP_DIR" \
      LC_ALL=C \
      LANG=C \
      FAKE_OS="$FAKE_OS" \
      FAKE_ARCH="$FAKE_ARCH" \
      FAKE_ROSETTA="$FAKE_ROSETTA" \
      FIXTURE_DIR="$FIXTURE_DIR" \
      DOWNLOAD_LOG="$DOWNLOAD_LOG" \
      TEST_FOX_VERSION="$ENV_VERSION" \
      TEST_FOX_VERSION_SET="$version_set" \
      TEST_FOX_INSTALL_DIR="$ENV_INSTALL_DIR" \
      TEST_FOX_INSTALL_DIR_SET="$install_dir_set" \
      TEST_FOX_NO_MODIFY_PATH="$ENV_NO_MODIFY_PATH" \
      TEST_FOX_NO_MODIFY_PATH_SET="$no_modify_path_set" \
      MUTATE_PROFILE_ON_CHECKSUM="$MUTATE_PROFILE_ON_CHECKSUM" \
      MUTATE_PROFILE_DURING_CMP="$MUTATE_PROFILE_DURING_CMP" \
      "$INSTALL_SHELL" -c '
        if [ "$TEST_FOX_VERSION_SET" -eq 1 ]; then
          FOX_VERSION=$TEST_FOX_VERSION
          export FOX_VERSION
        fi
        if [ "$TEST_FOX_INSTALL_DIR_SET" -eq 1 ]; then
          FOX_INSTALL_DIR=$TEST_FOX_INSTALL_DIR
          export FOX_INSTALL_DIR
        fi
        if [ "$TEST_FOX_NO_MODIFY_PATH_SET" -eq 1 ]; then
          FOX_NO_MODIFY_PATH=$TEST_FOX_NO_MODIFY_PATH
          export FOX_NO_MODIFY_PATH
        fi
        unset TEST_FOX_VERSION TEST_FOX_VERSION_SET
        unset TEST_FOX_INSTALL_DIR TEST_FOX_INSTALL_DIR_SET
        unset TEST_FOX_NO_MODIFY_PATH TEST_FOX_NO_MODIFY_PATH_SET
        exec "$@"
      ' fox-install-test "$INSTALL_SHELL" "$INSTALLER" "$@"
  ) >"$RUN_OUT" 2>"$RUN_ERR"
  RUN_STATUS=$?
  set -e
}

run_test() {
  name=$1
  shift
  TOTAL_COUNT=$((TOTAL_COUNT + 1))
  if ( "$@" ); then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf 'ok %d - %s\n' "$TOTAL_COUNT" "$name"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf 'not ok %d - %s\n' "$TOTAL_COUNT" "$name"
  fi
}

test_platform_mapping() {
  os=$1
  arch=$2
  rosetta=$3
  asset=$4
  setup_case curl
  FAKE_OS=$os
  FAKE_ARCH=$arch
  FAKE_ROSETTA=$rosetta
  run_installer --no-modify-path
  assert_status 0 || return 1
  assert_contains "$DOWNLOAD_LOG" "/releases/latest/download/fox_$asset.tar.gz" "latest archive mapping" || return 1
  assert_contains "$DOWNLOAD_LOG" "fox_$asset.tar.gz.sha256" "checksum mapping" || return 1
}

test_unsupported_os() {
  setup_case curl
  FAKE_OS=FreeBSD
  run_installer --no-modify-path
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Unsupported operating system: FreeBSD" "unsupported OS error" || return 1
  assert_no_download
}

test_unsupported_arch() {
  setup_case curl
  FAKE_ARCH=riscv64
  run_installer --no-modify-path
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Unsupported architecture: riscv64" "unsupported architecture error" || return 1
  assert_no_download
}

test_curl_preferred() {
  setup_case both
  run_installer --no-modify-path
  assert_status 0 || return 1
  if grep '^wget ' "$DOWNLOAD_LOG" >/dev/null 2>&1; then
    fail "wget was used even though curl was available"
  fi
  assert_contains "$DOWNLOAD_LOG" "curl https://" "curl preference"
}

test_wget_fallback() {
  setup_case wget
  run_installer --no-modify-path
  assert_status 0 || return 1
  assert_contains "$DOWNLOAD_LOG" "wget https://" "wget fallback"
}

test_missing_downloader() {
  setup_case none
  run_installer --no-modify-path
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "curl or wget is required" "missing downloader error" || return 1
  assert_no_download
}

test_version_precedence() {
  setup_case curl
  ENV_VERSION=v0.1.29
  run_installer --version v0.1.30 --no-modify-path
  assert_status 0 || return 1
  assert_contains "$DOWNLOAD_LOG" "/releases/download/v0.1.30/fox_linux_amd64.tar.gz" "CLI version precedence"
}

test_environment_version() {
  setup_case curl
  ENV_VERSION=v0.1.30
  run_installer --no-modify-path
  assert_status 0 || return 1
  assert_contains "$DOWNLOAD_LOG" "/releases/download/v0.1.30/fox_linux_amd64.tar.gz" "FOX_VERSION selection"
}

test_invalid_versions() {
  setup_case curl
  for version in 0.1.30 v0.1 v0.1.30-beta main 'v1.2.3/fox'
  do
    : >"$DOWNLOAD_LOG"
    run_installer --version "$version" --no-modify-path
    assert_nonzero || return 1
    assert_contains "$RUN_ERR" "Invalid fox version" "invalid version error" || return 1
    assert_no_download || return 1
  done

  embedded_newline=$(printf 'v1.2.3\njunk')
  carriage_return=$(printf '\r')
  trailing_newline=v1.2.3$TEST_LINE_FEED
  for version in "$embedded_newline" "$trailing_newline" "v1.2.3${carriage_return}junk"
  do
    : >"$DOWNLOAD_LOG"
    run_installer --version "$version" --no-modify-path
    assert_nonzero || return 1
    assert_contains "$RUN_ERR" "Invalid fox version" "line-break version error" || return 1
    assert_no_download || return 1
  done
}

test_empty_scalar_environment_values() {
  setup_case curl
  ENV_VERSION_SET=1
  run_installer --no-modify-path
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Invalid fox version" "empty FOX_VERSION error" || return 1
  assert_no_download || return 1

  ENV_VERSION=latest
  ENV_INSTALL_DIR_SET=1
  run_installer --no-modify-path
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Invalid installation directory" "empty FOX_INSTALL_DIR error" || return 1
  assert_no_download || return 1

  run_installer --version latest --install-dir "$CASE_ROOT/cli-bin" --no-modify-path
  assert_status 0
}

test_argument_errors_and_help() {
  setup_case curl
  run_installer --version
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Usage:" "missing value usage" || return 1
  assert_no_download || return 1

  run_installer --install-dir
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Usage:" "missing install directory usage" || return 1
  assert_no_download || return 1

  run_installer --unknown
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Unknown argument: --unknown" "unknown argument error" || return 1
  assert_no_download || return 1

  run_installer --help
  assert_status 0 || return 1
  assert_contains "$RUN_OUT" "FOX_VERSION" "help environment documentation" || return 1
  assert_contains "$RUN_OUT" "sh -s -- --version" "pipe-safe help example" || return 1
  assert_no_download
}

test_path_boolean_validation_and_override() {
  setup_case curl
  ENV_NO_MODIFY_PATH=maybe
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Invalid FOX_NO_MODIFY_PATH" "invalid boolean error" || return 1
  assert_no_download || return 1

  run_installer --no-modify-path
  assert_status 0 || return 1

  carriage_return=$(printf '\r')
  trailing_newline=true$TEST_LINE_FEED
  for ENV_NO_MODIFY_PATH in "$(printf 'true\njunk')" "$trailing_newline" "true${carriage_return}"
  do
    : >"$DOWNLOAD_LOG"
    run_installer
    assert_nonzero || return 1
    assert_contains "$RUN_ERR" "Invalid FOX_NO_MODIFY_PATH" "line-break boolean error" || return 1
    assert_no_download || return 1
  done
}

test_path_boolean_case_insensitive_values() {
  setup_case curl
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  ENV_NO_MODIFY_PATH=YES
  run_installer
  assert_status 0 || return 1
  assert_not_exists "$HOME_DIR/.bashrc" || return 1

  ENV_NO_MODIFY_PATH=FaLsE
  run_installer
  assert_status 0 || return 1
  assert_exists "$HOME_DIR/.bashrc"
}

test_checksum_mismatch_preserves_existing() {
  setup_case curl
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  ENV_NO_MODIFY_PATH=1
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  printf '%064d  fox_linux_amd64.tar.gz\n' 0 >"$FIXTURE_DIR/fox_linux_amd64.tar.gz.sha256"
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "checksum" "checksum mismatch error" || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox || return 1
  assert_directory_empty "$TMP_DIR"
}

test_malformed_checksum_preserves_existing() {
  setup_case curl
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  ENV_NO_MODIFY_PATH=1
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  printf 'not-a-checksum\nextra-line\n' >"$FIXTURE_DIR/fox_linux_amd64.tar.gz.sha256"
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "checksum file" "malformed checksum error" || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox
}

replace_fixture_with_extra_entry() {
  payload=$CASE_ROOT/unsafe-extra
  mkdir -p "$payload"
  printf 'fox-test-binary\n' >"$payload/fox"
  printf 'extra\n' >"$payload/extra"
  tar -C "$payload" -czf "$FIXTURE_DIR/fox_linux_amd64.tar.gz" fox extra
  write_checksum fox_linux_amd64.tar.gz
}

replace_fixture_with_symlink() {
  payload=$CASE_ROOT/unsafe-link
  mkdir -p "$payload"
  printf 'target\n' >"$payload/target"
  ln -s target "$payload/fox"
  tar -C "$payload" -czf "$FIXTURE_DIR/fox_linux_amd64.tar.gz" fox
  write_checksum fox_linux_amd64.tar.gz
}

replace_fixture_with_traversal() {
  payload=$CASE_ROOT/unsafe-traversal
  mkdir -p "$payload/sub"
  printf 'fox-test-binary\n' >"$payload/fox"
  tar -C "$payload" -czf "$FIXTURE_DIR/fox_linux_amd64.tar.gz" sub/../fox
  write_checksum fox_linux_amd64.tar.gz
}

test_unsafe_archive() {
  fixture_kind=$1
  setup_case curl
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  ENV_NO_MODIFY_PATH=1
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  case "$fixture_kind" in
    extra) replace_fixture_with_extra_entry ;;
    symlink) replace_fixture_with_symlink ;;
    traversal) replace_fixture_with_traversal ;;
    *) fail "unknown unsafe fixture: $fixture_kind" ;;
  esac
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "unsafe archive" "unsafe archive error" || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox
}

test_default_destination() {
  setup_case curl
  run_installer --no-modify-path
  assert_status 0 || return 1
  target=$HOME_DIR/.local/bin/fox
  assert_executable "$target" || return 1
  assert_content "$target" fox-test-binary || return 1
  assert_directory_empty "$TMP_DIR"
}

test_custom_destination_precedence_and_atomic_result() {
  setup_case curl
  ENV_INSTALL_DIR=$CASE_ROOT/env-bin
  cli_dir=$CASE_ROOT/cli-bin
  run_installer --install-dir "$cli_dir" --no-modify-path
  assert_status 0 || return 1
  assert_executable "$cli_dir/fox" || return 1
  assert_content "$cli_dir/fox" fox-test-binary || return 1
  assert_not_exists "$ENV_INSTALL_DIR/fox" || return 1
  if find "$cli_dir" -name '.fox.install.*' -print | grep . >/dev/null 2>&1; then
    fail "destination staging file was not cleaned up"
  fi
}

test_profile_mapping() {
  os=$1
  shell_path=$2
  expected_profile=$3
  setup_case curl
  FAKE_OS=$os
  CASE_SHELL=$shell_path
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  run_installer
  assert_status 0 || return 1
  assert_exists "$HOME_DIR/$expected_profile"
}

test_path_idempotency() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  run_installer
  assert_status 0 || return 1
  run_installer
  assert_status 0 || return 1
  profile=$HOME_DIR/.bashrc
  begin_count=$(grep -Fxc '# >>> foxharness installer >>>' "$profile" || true)
  end_count=$(grep -Fxc '# <<< foxharness installer <<<' "$profile" || true)
  assert_eq 1 "$begin_count" "duplicate PATH begin marker" || return 1
  assert_eq 1 "$end_count" "duplicate PATH end marker" || return 1
  canonical_dir=$(CDPATH= cd "$ENV_INSTALL_DIR" && pwd -P)
  actual_path=$(
    "$ENV_CMD" -i PATH=/base "$INSTALL_SHELL" -c '. "$1"; printf "%s" "$PATH"' sh "$profile"
  )
  assert_eq "$canonical_dir:/base" "$actual_path" "profile PATH result"
}

test_path_block_updates_directory() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/first-bin
  run_installer
  assert_status 0 || return 1

  ENV_INSTALL_DIR=$CASE_ROOT/second-bin
  run_installer
  assert_status 0 || return 1

  profile=$HOME_DIR/.bashrc
  begin_count=$(grep -Fxc '# >>> foxharness installer >>>' "$profile" || true)
  end_count=$(grep -Fxc '# <<< foxharness installer <<<' "$profile" || true)
  assert_eq 1 "$begin_count" "duplicate PATH begin marker after update" || return 1
  assert_eq 1 "$end_count" "duplicate PATH end marker after update" || return 1
  canonical_dir=$(CDPATH= cd "$ENV_INSTALL_DIR" && pwd -P)
  actual_path=$(
    "$ENV_CMD" -i PATH=/base "$INSTALL_SHELL" -c '. "$1"; printf "%s" "$PATH"' sh "$profile"
  )
  assert_eq "$canonical_dir:/base" "$actual_path" "updated profile PATH result"
}

make_tricky_directory_name() {
  tick=$(printf '\140')
  TRICKY_DIR="$CASE_ROOT/bin space \$HOME \$(touch PWNED) ' \" "
  TRICKY_DIR=$TRICKY_DIR$tick"touch PWNED2"$tick' \ end'
}

assert_no_injection_markers() {
  assert_not_exists "$TEST_CWD/PWNED" || return 1
  assert_not_exists "$TEST_CWD/PWNED2"
}

test_profile_path_serialization() {
  setup_case curl
  CASE_SHELL=/bin/bash
  make_tricky_directory_name
  ENV_INSTALL_DIR=$TRICKY_DIR
  run_installer
  assert_status 0 || return 1
  profile=$HOME_DIR/.bashrc
  assert_exists "$profile" || return 1
  canonical_dir=$(CDPATH= cd "$TRICKY_DIR" && pwd -P)
  actual_path=$(
    cd "$TEST_CWD"
    "$ENV_CMD" -i PATH=/base "$INSTALL_SHELL" -c '. "$1"; printf "%s" "$PATH"' sh "$profile"
  )
  assert_eq "$canonical_dir:/base" "$actual_path" "serialized profile PATH" || return 1
  assert_no_injection_markers
}

test_manual_path_serialization() {
  setup_case curl
  make_tricky_directory_name
  ENV_INSTALL_DIR=$TRICKY_DIR
  run_installer --no-modify-path
  assert_status 0 || return 1
  manual_line=$(grep '^export PATH=' "$RUN_OUT" | head -n 1)
  if [ -z "$manual_line" ]; then
    fail "manual export command was not printed"
    return 1
  fi
  canonical_dir=$(CDPATH= cd "$TRICKY_DIR" && pwd -P)
  actual_path=$(
    cd "$TEST_CWD"
    PATH=/base "$INSTALL_SHELL" -c "$manual_line"'; printf "%s" "$PATH"'
  )
  assert_eq "$canonical_dir:/base" "$actual_path" "serialized manual PATH" || return 1
  assert_no_injection_markers || return 1
  assert_not_exists "$HOME_DIR/.bashrc"
}

test_rejected_install_directory() {
  kind=$1
  setup_case curl
  ENV_NO_MODIFY_PATH=1
  case "$kind" in
    colon) ENV_INSTALL_DIR=$CASE_ROOT/bad:path ;;
    carriage_return) ENV_INSTALL_DIR=$CASE_ROOT/bad$(printf '\r')path ;;
    newline) ENV_INSTALL_DIR=$(printf '%s\n%s' "$CASE_ROOT/bad" path) ;;
    trailing_newline) ENV_INSTALL_DIR=$CASE_ROOT/bad$TEST_LINE_FEED ;;
    *) fail "unknown invalid directory kind: $kind" ;;
  esac
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Invalid installation directory" "invalid directory error" || return 1
  assert_no_download
}

test_rejected_canonical_install_directory() {
  kind=$1
  setup_case curl
  ENV_NO_MODIFY_PATH=1
  case "$kind" in
    colon) target_dir=$CASE_ROOT/physical:split ;;
    carriage_return) target_dir=$CASE_ROOT/physical$(printf '\r')split ;;
    newline) target_dir=$(printf '%s\n%s' "$CASE_ROOT/physical" split) ;;
    trailing_newline)
      target_without_newline=$CASE_ROOT/physical
      mkdir -p "$target_without_newline"
      target_dir=$target_without_newline$TEST_LINE_FEED
      ;;
    *) fail "unknown invalid canonical directory kind: $kind" ;;
  esac
  mkdir -p "$target_dir"
  safe_link=$CASE_ROOT/safe-link
  ln -s "$target_dir" "$safe_link"
  ENV_INSTALL_DIR=$safe_link
  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "Invalid installation directory" "invalid canonical directory error" || return 1
  assert_no_download
}

test_malformed_profile_preserves_existing() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  for fixture in unterminated reversed duplicate
  do
    case "$fixture" in
      unterminated)
        printf '# >>> foxharness installer >>>\n' >"$HOME_DIR/.bashrc"
        ;;
      reversed)
        printf '# <<< foxharness installer <<<\n# >>> foxharness installer >>>\n' >"$HOME_DIR/.bashrc"
        ;;
      duplicate)
        printf '# >>> foxharness installer >>>\n# <<< foxharness installer <<<\n# >>> foxharness installer >>>\n# <<< foxharness installer <<<\n' >"$HOME_DIR/.bashrc"
        ;;
    esac
    : >"$DOWNLOAD_LOG"
    run_installer
    assert_nonzero || return 1
    assert_contains "$RUN_ERR" "malformed foxharness installer block" "malformed profile error ($fixture)" || return 1
    assert_no_download || return 1
    assert_content "$ENV_INSTALL_DIR/fox" old-fox || return 1
  done
}

test_profile_symlink_preserves_existing() {
  kind=$1
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  profile=$HOME_DIR/.bashrc
  case "$kind" in
    regular)
      profile_target=$CASE_ROOT/managed-bashrc
      printf 'managed-profile\n' >"$profile_target"
      ln -s "$profile_target" "$profile"
      ;;
    dangling)
      profile_target=$CASE_ROOT/missing-bashrc
      ln -s "$profile_target" "$profile"
      ;;
    *) fail "unknown profile symlink kind: $kind" ;;
  esac

  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "shell profile is a symbolic link" "profile symlink error" || return 1
  assert_no_download || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox || return 1
  assert_symlink "$profile" || return 1
  if [ "$kind" = regular ]; then
    assert_content "$profile_target" managed-profile
  else
    assert_not_exists "$profile_target"
  fi
}

test_profile_revalidated_after_download() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  profile=$HOME_DIR/.bashrc
  MUTATE_PROFILE_ON_CHECKSUM=$profile

  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "malformed foxharness installer block" "post-download profile validation" || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox || return 1
  assert_content "$profile" '# >>> foxharness installer >>>'
}

test_concurrent_profile_change_is_preserved() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  mkdir -p "$ENV_INSTALL_DIR"
  printf 'old-fox\n' >"$ENV_INSTALL_DIR/fox"
  profile=$HOME_DIR/.bashrc
  printf 'managed-profile\n' >"$profile"
  MUTATE_PROFILE_DURING_CMP=$profile

  run_installer
  assert_nonzero || return 1
  assert_contains "$RUN_ERR" "shell profile changed during installation" "concurrent profile error" || return 1
  assert_content "$ENV_INSTALL_DIR/fox" old-fox || return 1
  assert_contains "$profile" "concurrent-profile-change" "concurrent profile content"
}

test_existing_path_is_not_modified() {
  setup_case curl
  CASE_SHELL=/bin/bash
  ENV_INSTALL_DIR=$CASE_ROOT/bin
  mkdir -p "$ENV_INSTALL_DIR"
  canonical_dir=$(CDPATH= cd "$ENV_INSTALL_DIR" && pwd -P)
  PROCESS_PATH=$canonical_dir:$FAKEBIN
  run_installer
  assert_status 0 || return 1
  assert_not_exists "$HOME_DIR/.bashrc"
}

create_suite_fixtures() {
  CASE_ROOT=$SUITE_ROOT/base-case
  BASE_FIXTURE_DIR=$SUITE_ROOT/base-fixtures
  FIXTURE_DIR=$BASE_FIXTURE_DIR
  mkdir -p "$CASE_ROOT" "$FIXTURE_DIR"
  create_valid_fixtures
}

create_suite_fixtures

run_test "Darwin x86_64 maps to darwin_amd64" test_platform_mapping Darwin x86_64 0 darwin_amd64
run_test "Darwin amd64 maps to darwin_amd64" test_platform_mapping Darwin amd64 0 darwin_amd64
run_test "Darwin arm64 maps to darwin_arm64" test_platform_mapping Darwin arm64 0 darwin_arm64
run_test "Darwin aarch64 maps to darwin_arm64" test_platform_mapping Darwin aarch64 0 darwin_arm64
run_test "Linux x86_64 maps to linux_amd64" test_platform_mapping Linux x86_64 0 linux_amd64
run_test "Linux amd64 maps to linux_amd64" test_platform_mapping Linux amd64 0 linux_amd64
run_test "Linux arm64 maps to linux_arm64" test_platform_mapping Linux arm64 0 linux_arm64
run_test "Linux aarch64 maps to linux_arm64" test_platform_mapping Linux aarch64 0 linux_arm64
run_test "Rosetta maps x86_64 to native arm64" test_platform_mapping Darwin x86_64 1 darwin_arm64
run_test "unsupported operating system is rejected" test_unsupported_os
run_test "unsupported architecture is rejected" test_unsupported_arch
run_test "curl is preferred over wget" test_curl_preferred
run_test "wget is used when curl is unavailable" test_wget_fallback
run_test "missing downloader fails before network access" test_missing_downloader
run_test "CLI version overrides FOX_VERSION" test_version_precedence
run_test "FOX_VERSION selects a pinned release" test_environment_version
run_test "invalid versions are rejected before download" test_invalid_versions
run_test "empty scalar environment values are rejected unless CLI overrides" test_empty_scalar_environment_values
run_test "argument errors and help are deterministic" test_argument_errors_and_help
run_test "PATH boolean validation honors CLI precedence" test_path_boolean_validation_and_override
run_test "PATH boolean values are case-insensitive" test_path_boolean_case_insensitive_values
run_test "checksum mismatch preserves an existing fox" test_checksum_mismatch_preserves_existing
run_test "malformed checksum preserves an existing fox" test_malformed_checksum_preserves_existing
run_test "archive with extra entry is rejected" test_unsafe_archive extra
run_test "archive with symlink fox is rejected" test_unsafe_archive symlink
run_test "archive with traversal name is rejected" test_unsafe_archive traversal
run_test "default destination is HOME/.local/bin" test_default_destination
run_test "custom destination precedence is atomic" test_custom_destination_precedence_and_atomic_result
run_test "macOS zsh selects .zprofile" test_profile_mapping Darwin /bin/zsh .zprofile
run_test "macOS bash selects .bash_profile" test_profile_mapping Darwin /bin/bash .bash_profile
run_test "Linux zsh selects .zshrc" test_profile_mapping Linux /bin/zsh .zshrc
run_test "Linux bash selects .bashrc" test_profile_mapping Linux /bin/bash .bashrc
run_test "other shells select .profile" test_profile_mapping Linux /bin/fish .profile
run_test "PATH profile update is idempotent" test_path_idempotency
run_test "PATH profile block updates a changed directory" test_path_block_updates_directory
run_test "profile PATH serialization prevents evaluation" test_profile_path_serialization
run_test "manual PATH serialization prevents evaluation" test_manual_path_serialization
run_test "colon installation directory is rejected" test_rejected_install_directory colon
run_test "carriage-return installation directory is rejected" test_rejected_install_directory carriage_return
run_test "newline installation directory is rejected" test_rejected_install_directory newline
run_test "trailing-newline installation directory is rejected" test_rejected_install_directory trailing_newline
run_test "symlink to colon installation directory is rejected" test_rejected_canonical_install_directory colon
run_test "symlink to carriage-return installation directory is rejected" test_rejected_canonical_install_directory carriage_return
run_test "symlink to newline installation directory is rejected" test_rejected_canonical_install_directory newline
run_test "symlink to trailing-newline directory is not redirected to its sibling" test_rejected_canonical_install_directory trailing_newline
run_test "malformed profile block preserves existing fox" test_malformed_profile_preserves_existing
run_test "regular profile symlink is preserved" test_profile_symlink_preserves_existing regular
run_test "dangling profile symlink is preserved" test_profile_symlink_preserves_existing dangling
run_test "profile markers are revalidated after download" test_profile_revalidated_after_download
run_test "concurrent profile changes are preserved" test_concurrent_profile_change_is_preserved
run_test "existing PATH element avoids profile mutation" test_existing_path_is_not_modified

printf '1..%d\n' "$TOTAL_COUNT"
printf '# pass %d\n' "$PASS_COUNT"
printf '# fail %d\n' "$FAIL_COUNT"

if [ "$FAIL_COUNT" -ne 0 ]; then
  exit 1
fi
