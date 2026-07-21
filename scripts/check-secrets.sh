#!/usr/bin/env sh
set -eu

pattern='(mysql|obclient)[[:space:]].*-[pP][^[:space:]]+|BEGIN[[:space:]]+(RSA[[:space:]]+|EC[[:space:]]+|OPENSSH[[:space:]]+)?PRIVATE[[:space:]]+KEY|AKIA[0-9A-Z]{16}'
matches_file=$(mktemp)
trap 'rm -f "$matches_file"' EXIT INT TERM
git ls-files --cached --others --exclude-standard -- cmd contracts internal migrations web .github | while IFS= read -r file; do
  if grep -nIH -E "$pattern" "$file" >> "$matches_file"; then
    :
  else
    status=$?
    if [ "$status" -ne 1 ]; then
      exit "$status"
    fi
  fi
done
if [ -s "$matches_file" ]; then
  printf 'Potential secret material found:\n' >&2
  cat "$matches_file" >&2
  exit 1
fi
printf 'Secret scan passed.\n'
