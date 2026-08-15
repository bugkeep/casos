#!/usr/bin/env bash
# CasOS one-step installer, adapted from OpenAgent's Apache-2.0 installer.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/casosorg/casos/master/scripts/install.sh | bash -s -- --uninstall
#
# Optional environment variables:
#   CASOS_VERSION  release tag such as v1.32.0 (default: latest)
#   CASOS_REPOSITORY GitHub release repository (default: casosorg/casos)
#   INSTALL_DIR    binary directory (default: $HOME/.local/bin)

set -euo pipefail

REPO="${CASOS_REPOSITORY:-casosorg/casos}"
CASOS_VERSION="${CASOS_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

info() { printf '%s\n' "$*"; }
die() { printf '[casos] %s\n' "$*" >&2; exit 1; }

usage() {
	cat <<'EOF'
CasOS installer.

  --uninstall   remove the CasOS binary and its PATH entry
  --help        show this message

Piping the script to bash needs "-s --" before the option:
  curl -fsSL .../install.sh | bash -s -- --uninstall
EOF
}

ACTION="install"
while [[ $# -gt 0 ]]; do
	case "$1" in
		--uninstall) ACTION="uninstall" ;;
		-h|--help) usage; exit 0 ;;
		*) die "unknown option: $1 (try --help)" ;;
	esac
	shift
done

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

[[ "$INSTALL_DIR" != *:* ]] || die "INSTALL_DIR must not contain a PATH separator (:)"

# The line install adds to the shell rc file, and the file it goes in. Both
# steps have to agree exactly so that uninstall can find and drop the line.
shell_rc_path() {
	local login_shell="${SHELL:-}" detected
	if command -v getent >/dev/null 2>&1; then
		detected="$(getent passwd "$(id -un)" | cut -d: -f7 || true)"
		[[ -z "$detected" ]] || login_shell="$detected"
	fi
	case "$login_shell" in
		*/zsh) printf '%s\n' "$HOME/.zshrc" ;;
		*/bash) printf '%s\n' "$HOME/.bashrc" ;;
	esac
}

path_line_for() { printf 'export PATH=%q:"$PATH"' "$1"; }

# Mirrors conf.defaultDataDir so uninstall can report where state was left.
data_dir() {
	case "$(uname -s)" in
		Darwin) printf '%s\n' "$HOME/Library/Application Support/CasOS" ;;
		*)
			if [[ "${XDG_DATA_HOME:-}" == /* ]]; then
				printf '%s\n' "$XDG_DATA_HOME/casos"
			else
				printf '%s\n' "$HOME/.local/share/casos"
			fi
			;;
	esac
}

if [[ "$ACTION" == "uninstall" ]]; then
	# Resolve without creating anything: uninstall must not make directories.
	[[ ! -d "$INSTALL_DIR" ]] || INSTALL_DIR="$(cd "$INSTALL_DIR" && pwd -P)"
	FOUND=0

	if [[ -e "$INSTALL_DIR/casos" ]]; then
		rm -f "$INSTALL_DIR/casos" || die "could not remove $INSTALL_DIR/casos"
		info "Removed $INSTALL_DIR/casos"
		FOUND=1
	else
		info "No CasOS binary at $INSTALL_DIR/casos"
	fi

	SHELL_RC="$(shell_rc_path)"
	PATH_LINE="$(path_line_for "$INSTALL_DIR")"
	if [[ -n "$SHELL_RC" ]] && [[ -f "$SHELL_RC" ]] && grep -Fxq "$PATH_LINE" "$SHELL_RC"; then
		RC_TEMP="$(mktemp)"
		grep -Fxv "$PATH_LINE" "$SHELL_RC" > "$RC_TEMP" || true
		# Write through the original file so its permissions, ownership and any
		# symlink pointing at it survive the edit.
		cat "$RC_TEMP" > "$SHELL_RC"
		rm -f "$RC_TEMP"
		info "Removed the CasOS PATH entry from $SHELL_RC"
		FOUND=1
	fi

	[[ "$FOUND" -eq 1 ]] || info "Nothing to uninstall."

	DATA_DIR="$(data_dir)"
	if [[ -d "$DATA_DIR" ]]; then
		info ""
		info "Your CasOS data was left in place at:"
		info "  $DATA_DIR"
		info "It holds the SQLite databases, the control-plane TLS material and the"
		info "key that decrypts stored SSH credentials. Delete it only if you are"
		info "certain you want that state gone:"
		info "  rm -rf $(printf '%q' "$DATA_DIR")"
	fi
	exit 0
fi

[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "invalid GitHub repository: $REPO"

need_cmd curl

if [[ "$CASOS_VERSION" == "latest" ]]; then
	RELEASE_URL="https://github.com/${REPO}/releases/latest/download"
else
	[[ "$CASOS_VERSION" =~ ^v[0-9A-Za-z._-]+$ ]] || die "invalid release version: $CASOS_VERSION"
	RELEASE_URL="https://github.com/${REPO}/releases/download/${CASOS_VERSION}"
fi

case "$(uname -s)" in
	Linux) OS_NAME="linux" ;;
	Darwin) OS_NAME="darwin" ;;
	*) die "unsupported operating system; download manually from https://github.com/${REPO}/releases" ;;
esac

case "$(uname -m)" in
	x86_64|amd64) ARCH_NAME="x86_64" ;;
	aarch64|arm64) ARCH_NAME="arm64" ;;
	*) die "unsupported architecture; download manually from https://github.com/${REPO}/releases" ;;
esac

# Under Rosetta 2 `uname -m` describes the emulated x86_64 shell rather than the
# Apple Silicon host, so install the native arm64 build instead of an emulated one.
if [[ "$OS_NAME" == "darwin" && "$ARCH_NAME" == "x86_64" ]] &&
	[[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" == "1" ]]; then
	ARCH_NAME="arm64"
fi

FILENAME="casos_${OS_NAME}_${ARCH_NAME}.tar.gz"
TEMP_DIR="$(mktemp -d)"
PENDING=""
cleanup() {
	rm -rf "$TEMP_DIR"
	[[ -z "$PENDING" ]] || rm -f "$PENDING"
}
trap cleanup EXIT

info "Downloading CasOS ${CASOS_VERSION}..."
curl -fsSL -o "$TEMP_DIR/$FILENAME" "$RELEASE_URL/$FILENAME"
curl -fsSL -o "$TEMP_DIR/SHA256SUMS" "$RELEASE_URL/SHA256SUMS"

EXPECTED="$(grep -E "^[0-9a-fA-F]{64}[[:space:]]+\\*?${FILENAME}$" "$TEMP_DIR/SHA256SUMS" | head -1 | awk '{print tolower($1)}' || true)"
[[ -n "$EXPECTED" ]] || die "release checksum for $FILENAME was not found"
if command -v sha256sum >/dev/null 2>&1; then
	ACTUAL="$(sha256sum "$TEMP_DIR/$FILENAME" | awk '{print tolower($1)}')"
elif command -v shasum >/dev/null 2>&1; then
	ACTUAL="$(shasum -a 256 "$TEMP_DIR/$FILENAME" | awk '{print tolower($1)}')"
else
	die "required command not found: sha256sum or shasum"
fi
[[ "$ACTUAL" == "$EXPECTED" ]] || die "download checksum verification failed"

# The release ships the executable compressed, which cuts the download to about
# a third. The archive holds that one file and nothing else.
need_cmd tar
tar -xzf "$TEMP_DIR/$FILENAME" -C "$TEMP_DIR" casos ||
	die "could not extract casos from $FILENAME"
[[ -f "$TEMP_DIR/casos" ]] || die "$FILENAME did not contain the casos executable"

mkdir -p "$INSTALL_DIR"
INSTALL_DIR="$(cd "$INSTALL_DIR" && pwd -P)"
[[ -w "$INSTALL_DIR" ]] || die "installation directory is not writable: $INSTALL_DIR"

PENDING="$INSTALL_DIR/.casos.new.$$"
cp "$TEMP_DIR/casos" "$PENDING"
chmod 755 "$PENDING"
mv -f "$PENDING" "$INSTALL_DIR/casos"
PENDING=""

SHELL_RC="$(shell_rc_path)"
PATH_LINE="$(path_line_for "$INSTALL_DIR")"
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]] && [[ -n "$SHELL_RC" ]] && \
	! grep -Fq "$PATH_LINE" "$SHELL_RC" 2>/dev/null; then
	printf '\n%s\n' "$PATH_LINE" >> "$SHELL_RC"
	info "Added $INSTALL_DIR to PATH in $SHELL_RC."
fi

info "CasOS ${CASOS_VERSION} installed at $INSTALL_DIR/casos"
if [[ ":$PATH:" == *":$INSTALL_DIR:"* ]]; then
	info "Run 'casos' and open http://localhost:9000"
elif [[ -n "$SHELL_RC" ]]; then
	SOURCE_COMMAND="$(printf 'source %q' "$SHELL_RC")"
	info "Open a new shell or run: $SOURCE_COMMAND"
	info "Then run 'casos' and open http://localhost:9000"
	info "You can also run '$INSTALL_DIR/casos' directly."
else
	info "Add $INSTALL_DIR to PATH for your shell, or run '$INSTALL_DIR/casos' directly."
	info "Then open http://localhost:9000"
fi
