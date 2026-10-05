#!/usr/bin/env bash
# Sourced by check-prod-container-verify-regression.sh; shares its fake Docker.
# Exercise the actual cleanup function as an EXIT trap, never a real stack.
fixture="${fixture:?parent regression fixture required}"
echo "--- Test 17: Failed teardown exit status and diagnostics ---"
awk '/^cleanup_stack\(\) \{/ { inside = 1 } inside { print } inside && /^}/ { exit }' \
    "$SCRIPT" >"$fixture/cleanup.sh"
for primary in 0 73; do
    managed_tmp=$(mktemp -d "$fixture/managed.XXXXXX")
    touch "$managed_tmp/override.yaml"
    calls="$fixture/cleanup-$primary.calls"
    diagnostic="$fixture/cleanup-$primary.log"
    project="geoguessme-prod-verify-regression-cleanup-$primary"
    status=0
    PATH="$fixture:$PATH" PROD_VERIFY_DOWN_FAIL=1 PROD_VERIFY_CALLS="$calls" \
        GEOGUESSME_PROD_VERIFY_PROJECT="$project" PROJECT="$project" \
        TMPDIR="$managed_tmp" REPO="$(dirname "$COMPOSE_PROD")/.." \
        backend_image=fixture-backend web_image=fixture-web \
        bash -c 'source "$1"; trap cleanup_stack EXIT; exit "$2"' \
        _ "$fixture/cleanup.sh" "$primary" >"$diagnostic" 2>&1 || status=$?
    expected_status="$primary"
    if [ "$primary" -eq 0 ]; then expected_status=42; fi
    if [ "$status" -eq "$expected_status" ] &&
        [ "$(<"$calls")" = "down $project $managed_tmp/override.yaml" ] &&
        [ ! -e "$managed_tmp" ] &&
        grep -q 'simulated down failure diagnostic' "$diagnostic" &&
        grep -q "teardown of managed project $project failed" "$diagnostic"; then
        pass "down failure is visible; primary=$primary exits $expected_status and removes temp files"
    else
        fail "failed teardown masks primary status or suppresses diagnostics/cleanup"
        cat "$diagnostic"
    fi
done

# Only these subprocesses use fake rm; the outer EXIT trap uses real rm.
mkdir "$fixture/rm-failure"
printf '#!/usr/bin/env bash\necho "simulated rm failure diagnostic" >&2\nexit 43\n' >"$fixture/rm-failure/rm"
chmod +x "$fixture/rm-failure/rm"
for primary in 0 73; do
    managed_tmp=$(mktemp -d "$fixture/managed.XXXXXX")
    touch "$managed_tmp/override.yaml"
    calls="$fixture/rm-$primary.calls"
    diagnostic="$fixture/rm-$primary.log"
    project="geoguessme-prod-verify-regression-rm-$primary"
    status=0
    PATH="$fixture/rm-failure:$fixture:$PATH" PROD_VERIFY_CALLS="$calls" \
        GEOGUESSME_PROD_VERIFY_PROJECT="$project" PROJECT="$project" \
        TMPDIR="$managed_tmp" REPO="$(dirname "$COMPOSE_PROD")/.." \
        backend_image=fixture-backend web_image=fixture-web \
        bash -c 'source "$1"; trap cleanup_stack EXIT; exit "$2"' \
        _ "$fixture/cleanup.sh" "$primary" >"$diagnostic" 2>&1 || status=$?
    expected_status="$primary"
    if [ "$primary" -eq 0 ]; then expected_status=1; fi
    if [ "$status" -eq "$expected_status" ] && [ -d "$managed_tmp" ] &&
        [ "$(<"$calls")" = "down $project $managed_tmp/override.yaml" ] &&
        grep -q 'simulated rm failure diagnostic' "$diagnostic" &&
        grep -q 'removing verification temporary files failed' "$diagnostic"; then
        pass "rm failure is visible; primary=$primary exits $expected_status"
    else
        fail "temporary cleanup masks primary status or suppresses diagnostics"
        cat "$diagnostic"
    fi
done
