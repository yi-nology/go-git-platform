#!/usr/bin/env bash
# 四域覆盖率断言（spec §1.1/§6.1）：任一域 <80% 非零退出。make check 调用。
set -euo pipefail
OUT="${1:-coverage.out}"
[ -f "$OUT" ] || { echo "cover-domain: $OUT missing — run 'go test -coverprofile=$OUT ./...' first" >&2; exit 2; }
fail=0
check() { # name regex
  local name="$1" rx="$2"
  local agg
  agg=$(go tool cover -func="$OUT" | grep -E "$rx" | awk -F'\t+' '{gsub("%","",$NF); s+=$NF; n++} END {if(n) printf "%.1f", s/n; else print "NA"}')
  if [ "$agg" = "NA" ]; then echo "cover-domain: $name NO FUNCS MATCHED ($rx)" >&2; fail=1; return; fi
  local ok
  ok=$(awk -v a="$agg" 'BEGIN{print (a+0 >= 80) ? 1 : 0}')
  if [ "$ok" = "1" ]; then echo "cover-domain: $name ${agg}% OK"; else echo "cover-domain: $name ${agg}% < 80%" >&2; fail=1; fi
}
check "webhook"   'runtime/webhook|provider/webhook'
check "transport" 'transport/(retry|etag|client)\.go:'
check "gitbackend" 'gitbackend/'
check "pageiter"  'pageiter'
exit $fail
