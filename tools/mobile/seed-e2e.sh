#!/usr/bin/env bash
set -euo pipefail

base_url=${1:?base URL is required}
output_file=${2:?output file is required}
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
run_id=$(date +%s)-$RANDOM
uploader="mobile_u_${run_id//-/}"
player="mobile_p_${run_id//-/}"
password="Mobile-${run_id}-Aa1"
group_name="Android E2E $run_id"

signup() {
    local username=$1
    curl --fail --silent --show-error \
        -H 'Content-Type: application/json' \
        --data "{\"username\":\"$username\",\"email\":\"$username@example.test\",\"password\":\"$password\"}" \
        "$base_url/api/v1/auth/signup"
}

uploader_response=$(signup "$uploader")
player_response=$(signup "$player")
uploader_token=$(jq -er '.access_token' <<<"$uploader_response")
player_token=$(jq -er '.access_token' <<<"$player_response")

group_response=$(curl --fail --silent --show-error \
    -H "Authorization: Bearer $uploader_token" -H 'Content-Type: application/json' \
    --data "{\"name\":\"$group_name\"}" "$base_url/api/v1/group/create")
group_id=$(jq -er '.id' <<<"$group_response")
invite_response=$(curl --fail --silent --show-error \
    -H "Authorization: Bearer $uploader_token" -H 'Content-Type: application/json' \
    --data "{\"group_id\":\"$group_id\"}" "$base_url/api/v1/group/invites")
invite_token=$(jq -er '.token' <<<"$invite_response")
curl --fail --silent --show-error \
    -H "Authorization: Bearer $player_token" -H 'Content-Type: application/json' \
    --data "{\"invite_token\":\"$invite_token\"}" "$base_url/api/v1/group/join" >/dev/null

curl --fail --silent --show-error \
    -H "Authorization: Bearer $uploader_token" \
    -F "photo=@$repo/frontend/public/logo.png;type=image/png" \
    -F "group_ids=$group_id" -F 'hide_location=false' -F 'lat=48.8566' -F 'long=2.3522' \
    "$base_url/api/v1/photo/upload" >/dev/null

umask 077
{
    printf 'MOBILE_USERNAME=%q\n' "$player"
    printf 'MOBILE_PASSWORD=%q\n' "$password"
    printf 'MOBILE_GROUP_NAME=%q\n' "$group_name"
} >"$output_file"

echo "Seeded isolated mobile fixture group: $group_name"
