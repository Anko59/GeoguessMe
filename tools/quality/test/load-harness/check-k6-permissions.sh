#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "$0")/../../../.." && pwd)
uid=${TOOLS_UID:-$(id -u)}
gid=${TOOLS_GID:-$(id -g)}
[[ "$uid" =~ ^[1-9][0-9]*$ && "$gid" =~ ^(0|[1-9][0-9]*)$ ]] || {
    echo 'k6 permission regression requires canonical non-root owner mapping' >&2
    exit 2
}
image=grafana/k6:0.55.0@sha256:b24f418fc99a26dd57904c952c03bfaf79462be18508acc45aafa07ff68e7df2
fixture=$(mktemp -d)
trap 'rm -rf "${fixture:?}"' EXIT
chmod 0700 "$fixture"
cp "$root/tools/load/k6.js" "$fixture/k6.js"
chmod 0600 "$fixture/k6.js"
# Inspect compiles the real unchanged scenario/options, but never runs setup,
# HTTP, WebSockets or thresholds against a server. Network access is disabled.
image_uid=$(docker run --rm --network none --read-only --cap-drop ALL \
    --security-opt no-new-privileges --entrypoint id "$image" -u)
status=0
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --mount "type=bind,src=$fixture,dst=/fixture,readonly" --workdir /fixture \
    "$image" inspect /fixture/k6.js >"$fixture/default.log" 2>&1 || status=$?
if [ "$image_uid" != "$uid" ]; then
    test "$status" -eq 255
    grep -q 'stat .: permission denied' "$fixture/default.log"
else
    test "$status" -eq 0
fi
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges \
    --user "$uid:$gid" --mount "type=bind,src=$fixture,dst=/fixture,readonly" \
    --workdir /fixture "$image" inspect /fixture/k6.js >"$fixture/mapped.log"
for threshold in http_req_duration http_req_failed websocket_delivery_failures rate_limit_enforced; do
    grep -q "\"$threshold\"" "$fixture/mapped.log"
done
for value in '"vus": 5' '"duration": "30s"' '"p(95)\u003c500"' \
    '"rate\u003c0.01"' '"rate==0"' '"count\u003e=1"'; do
    grep -Fq "$value" "$fixture/mapped.log"
done
cmp "$root/tools/load/k6.js" "$fixture/k6.js"
test "$(stat -c %a "$fixture")" = 700
test "$(stat -c %a "$fixture/k6.js")" = 600
echo "actual pinned k6 permission regression PASSED: image UID=$image_uid mapped UID=$uid:$gid, network-none inspect, restrictive fixture/profile unchanged"
