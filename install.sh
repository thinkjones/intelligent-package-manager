#!/usr/bin/env bash
# Installs ipm into ~/.local/bin (or $IPM_BIN_DIR), then starts `ipm onboard`.
#
#   ./install.sh                     from a checkout: builds it
#   curl -fsSL https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/install.sh | bash
#                                    anywhere else: go install @latest
#
# Needs Go. ipm keeps its data in your user folders; `ipm uninstall` removes it.
set -euo pipefail

module="github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm"
bin_dir="${IPM_BIN_DIR:-$HOME/.local/bin}"
target="$bin_dir/ipm"

if ! command -v go >/dev/null 2>&1; then
  echo "ipm is built with Go — install it from https://go.dev/dl, then rerun this script" >&2
  exit 1
fi

mkdir -p "$bin_dir"
here="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || true)"
if [[ -n $here && -f $here/cli/go.mod ]]; then
  echo "building ipm from $here/cli"
  (cd "$here/cli" && go build -o "$target" ./cmd/ipm)
else
  echo "installing $module@latest"
  GOBIN="$bin_dir" go install "$module@latest"
fi
echo "✓ installed $target ($("$target" version))"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *)
    case "${SHELL##*/}" in
      zsh) rc="$HOME/.zshrc" ;;
      bash) rc="$HOME/.bashrc" ;;
      *) rc="$HOME/.profile" ;;
    esac
    echo "! $bin_dir is not on your PATH — add this line to $rc:"
    echo "    export PATH=\"$bin_dir:\$PATH\""
    ;;
esac
found="$(command -v ipm 2>/dev/null || true)"
if [[ -n $found && $found != "$target" ]]; then
  echo "! another ipm comes first on your PATH: $found"
fi

if [[ -t 0 && -t 1 ]]; then
  exec "$target" onboard
fi
echo "next: ipm onboard"
