#!/usr/bin/env bash
set -euo pipefail

input="${1:-}"
if [[ -z "${input}" ]]; then
  echo "Usage: $0 <version-or-tag>" >&2
  echo "Example: $0 0.1.0   or   $0 v0.1.0" >&2
  exit 1
fi

if [[ "${input}" == v* ]]; then
  tag="${input}"
  version="${input#v}"
else
  version="${input}"
  tag="v${input}"
fi

url="https://github.com/benithors/dothuntcli/archive/refs/tags/${tag}.tar.gz"
tmp="$(mktemp /tmp/dothuntcli-${tag}-XXXXXX.tar.gz)"

cleanup() {
  rm -f "${tmp}"
}
trap cleanup EXIT

curl -fLsS -o "${tmp}" "${url}"
sha256="$(shasum -a 256 "${tmp}" | awk '{print $1}')"

cat <<EOF
version "${version}"
url "${url}"
sha256 "${sha256}"
EOF
