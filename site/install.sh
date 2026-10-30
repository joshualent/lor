#!/bin/sh
# Lor installer (https://lor.hyperf.app). Downloads a release from
# https://github.com/joshualent/lor/releases and checks it against the
# release's SHA256SUMS.
#
#   Agent CLI:   curl -fsSL https://lor.hyperf.app/install.sh | sh
#   Server:      curl -fsSL https://lor.hyperf.app/install.sh | sudo sh -s -- --server
#
# Options:
#   --server          install lor-server and its systemd unit (Linux, as root)
#   --skill           also install the agent skill to ~/.claude/skills/lor-access/
#   --version vX.Y.Z  install that release instead of the latest
#   --dir DIR         where to put the binary (default: /usr/local/bin as root
#                     or when writable, otherwise ~/.local/bin)
#
# Everything runs from main() at the bottom, so a partial download does nothing.
set -eu

REPO="joshualent/lor"

say() { printf '%s\n' "$*"; }
step() {
	if [ -t 1 ]; then
		printf '\033[1;34m==>\033[0m %s\n' "$*"
	else
		printf '==> %s\n' "$*"
	fi
}
die() {
	printf 'lor-install: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<'USAGE'
Lor installer (https://lor.hyperf.app)

  curl -fsSL https://lor.hyperf.app/install.sh | sh                        # agent CLI
  curl -fsSL https://lor.hyperf.app/install.sh | sudo sh -s -- --server    # server

Options:
  --server          install lor-server and its systemd unit (Linux, as root)
  --skill           also install the agent skill to ~/.claude/skills/lor-access/
  --version vX.Y.Z  install that release instead of the latest
  --dir DIR         where to put the binary (default: /usr/local/bin as root
                    or when writable, otherwise ~/.local/bin)
USAGE
}

fetch() { # url dest
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q --https-only -O "$2" "$1"
	else
		die "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "needs sha256sum or shasum to verify the download"
	fi
}

detect_platform() {
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "unsupported OS $(uname -s) (Lor runs on Linux and macOS)" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) die "unsupported CPU $(uname -m) (Lor builds for amd64 and arm64)" ;;
	esac
}

# download name: fetches and verifies the archive, then unpacks it into $tmp/x.
download() {
	if [ "$version" = latest ]; then
		base="https://github.com/$REPO/releases/latest/download"
	else
		base="https://github.com/$REPO/releases/download/$version"
	fi
	archive="${1}_${os}_${arch}.tar.gz"
	step "Downloading $archive ($version)"
	fetch "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"
	fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || die "download failed: $base/SHA256SUMS"
	want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
	[ -n "$want" ] || die "$archive is not listed in SHA256SUMS"
	got=$(sha256 "$tmp/$archive")
	[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"
	mkdir -p "$tmp/x"
	tar -xzf "$tmp/$archive" -C "$tmp/x"
}

default_dir() {
	if [ "$(id -u)" -eq 0 ] || [ -w /usr/local/bin ]; then
		echo /usr/local/bin
	else
		echo "$HOME/.local/bin"
	fi
}

install_skill() {
	[ -n "${HOME:-}" ] || die "--skill needs \$HOME"
	skill_dir="$HOME/.claude/skills/lor-access"
	mkdir -p "$skill_dir"
	cp "$tmp/x/SKILL.md" "$skill_dir/SKILL.md"
	chmod 0644 "$skill_dir/SKILL.md"
	step "Installed the agent skill to $skill_dir/SKILL.md"
}

install_cli() {
	download lor
	dir=${dir:-$(default_dir)}
	mkdir -p "$dir"
	install -m 0755 "$tmp/x/lor" "$dir/lor"
	step "Installed $("$dir/lor" --version 2>/dev/null || echo lor) to $dir/lor"
	if [ "$skill" = 1 ]; then
		install_skill
	fi
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "" && say "$dir is not on your PATH. Add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
	esac
	say ""
	say "Next: in Lor, open Agents > Add agent and run the 'lor enroll ...' command it shows."
}

install_server() {
	[ "$os" = linux ] || die "lor-server runs on Linux only"
	[ "$(id -u)" -eq 0 ] || die "--server needs root (pipe into 'sudo sh -s -- --server')"
	command -v systemctl >/dev/null 2>&1 || die "--server needs systemd"
	download lor-server
	dir=${dir:-/usr/local/bin}
	was_active=0
	if systemctl is-active --quiet lor-server 2>/dev/null; then
		was_active=1
	fi
	install -m 0755 "$tmp/x/lor-server" "$dir/lor-server"
	step "Installed $("$dir/lor-server" --version 2>/dev/null || echo lor-server) to $dir/lor-server"

	if ! id lor >/dev/null 2>&1; then
		useradd --system --home /var/lib/lor --shell /usr/sbin/nologin lor
		step "Created the lor system user"
	fi
	install -d -m 0755 /etc/lor
	new_config=0
	if [ ! -e /etc/lor/lor-server.toml ]; then
		install -m 0644 "$tmp/x/lor-server.toml" /etc/lor/lor-server.toml
		new_config=1
		step "Wrote the example config to /etc/lor/lor-server.toml"
	fi
	if [ ! -e /etc/systemd/system/lor-server.service ]; then
		sed "s|/usr/local/bin/lor-server|$dir/lor-server|" "$tmp/x/lor-server.service" >/etc/systemd/system/lor-server.service
		chmod 0644 /etc/systemd/system/lor-server.service
		systemctl daemon-reload 2>/dev/null || true
		step "Installed the systemd unit lor-server.service"
	fi

	say ""
	if [ "$was_active" = 1 ]; then
		systemctl restart lor-server
		step "Restarted lor-server"
	elif [ "$new_config" = 1 ]; then
		say "Next:"
		say "  1. Edit /etc/lor/lor-server.toml (public_url, admin_listen, agent_listen, trusted_proxy)."
		say "  2. systemctl enable --now lor-server"
		say "  3. sudo -u lor $dir/lor-server admin setup-code --config /etc/lor/lor-server.toml"
		say "The full guide: https://github.com/$REPO#deployment"
	else
		say "Start it with: systemctl enable --now lor-server"
	fi
}

main() {
	server=0
	skill=0
	version=latest
	dir=
	while [ $# -gt 0 ]; do
		case "$1" in
		--server) server=1 ;;
		--skill) skill=1 ;;
		--version)
			[ $# -ge 2 ] || die "--version needs a value"
			version=$2
			shift
			;;
		--version=*) version=${1#*=} ;;
		--dir)
			[ $# -ge 2 ] || die "--dir needs a value"
			dir=$2
			shift
			;;
		--dir=*) dir=${1#*=} ;;
		-h | --help)
			usage
			exit 0
			;;
		*) die "unknown option $1 (try --help)" ;;
		esac
		shift
	done
	case "$version" in
	latest | v*) ;;
	*) version="v$version" ;;
	esac

	detect_platform
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT
	trap 'exit 130' INT TERM

	if [ "$server" = 1 ]; then
		[ "$skill" = 0 ] || die "--skill is for the agent CLI; leave it off with --server"
		install_server
	else
		install_cli
	fi
}

main "$@"
