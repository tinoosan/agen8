#!/bin/sh
set -eu

major=""
if command -v node >/dev/null 2>&1; then
	version="$(node -v 2>/dev/null || true)"
	case "$version" in
		v*)
			major="${version#v}"
			major="${major%%.*}"
			;;
	esac
fi

case "$major" in
	20|21|22|23|24)
		exec npm "$@"
		;;
	*)
		echo "web-npm: host Node major ${major:-unknown} is outside the preferred range; continuing with host npm. See .nvmrc for the pinned local version." >&2
		exec npm "$@"
		;;
esac
