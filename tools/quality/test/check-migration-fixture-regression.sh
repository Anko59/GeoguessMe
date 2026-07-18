#!/usr/bin/env bash
# Regression checks for the deterministic migration fixture and test script.
# Verifies fixture coverage, script references, and structural invariants
# without a live database.  The full integration path is exercised by
# 'make migration-test'.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$REPO"

FIXTURE="deployment/scripts/legacy-migration-fixture.sql"
SCRIPT="deployment/scripts/migration-concurrency.sh"

failures=0
pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*"; failures=$((failures + 1)); }

# -- fixture file exists and is non-empty ------------------------------------
if [ -s "$FIXTURE" ]; then
    pass "legacy fixture exists and is non-empty"
else
    fail "legacy fixture missing or empty: $FIXTURE"
fi

# -- script exists and is executable -----------------------------------------
if [ -x "$SCRIPT" ]; then
    pass "migration-concurrency.sh exists and is executable"
else
    fail "migration-concurrency.sh missing or not executable: $SCRIPT"
fi

# -- fixture defines all expected legacy tables ------------------------------
for tbl in users groups group_members photos guesses messages; do
    if grep -q "CREATE TABLE $tbl" "$FIXTURE"; then
        pass "fixture defines legacy table '$tbl'"
    else
        fail "fixture missing legacy table '$tbl'"
    fi
done

# -- fixture must NOT define tables that migrations create from scratch -------
for tbl in challenge_views refresh_sessions email_verification_tokens password_reset_tokens websocket_tickets media_deletion_jobs schema_migrations; do
    if ! grep -q "CREATE TABLE.*$tbl" "$FIXTURE"; then
        pass "fixture correctly omits migration-created table '$tbl'"
    else
        fail "fixture must not create table '$tbl' (migrations create it from scratch)"
    fi
done

# -- fixture has data for every legacy table ----------------------------------
for tbl in users groups group_members photos guesses messages; do
    if grep -q "INSERT INTO $tbl" "$FIXTURE"; then
        pass "fixture inserts rows into '$tbl'"
    else
        fail "fixture has no INSERT for '$tbl'"
    fi
done

# -- script references the fixture --------------------------------------------
if grep -q "legacy-migration-fixture.sql" "$SCRIPT"; then
    pass "test script references the fixture file"
else
    fail "test script does not reference the fixture file"
fi

# -- script asserts on expected backfill labels --------------------------------
for label in \
    "email backfill" \
    "email_normalized" \
    "score column dropped" \
    "storage_key" \
    "group_id backfilled from photo" \
    "kind default" \
    "schema_migrations entries after concurrent run" \
    "duplicate survivor" \
    "ON CONFLICT DO NOTHING"; do
    if grep -q "$label" "$SCRIPT"; then
        pass "script asserts: '$label'"
    else
        fail "script missing assertion for: '$label'"
    fi
done

# -- fixture must not create schema_migrations (test proves it's absent) ------
if grep -qE "CREATE TABLE.*schema_migrations|INSERT INTO schema_migrations" "$FIXTURE"; then
    fail "fixture creates or inserts into schema_migrations (must not pre-create it)"
else
    pass "fixture does not create or insert into schema_migrations"
fi

# -- Makefile targets point to the script -------------------------------------
if grep -q "migration-concurrency.sh" Makefile; then
    pass "Makefile migration-test references migration-concurrency.sh"
else
    fail "Makefile migration-test does not reference migration-concurrency.sh"
fi

# -- structure: fixture line count within limit -------------------------------
fixture_lines=$(wc -l < "$FIXTURE")
if [ "$fixture_lines" -le 500 ]; then
    pass "fixture is $fixture_lines lines (≤ 500)"
else
    fail "fixture is $fixture_lines lines (exceeds 500-line limit)"
fi

# -- structure: script line count within limit --------------------------------
script_lines=$(wc -l < "$SCRIPT")
if [ "$script_lines" -le 500 ]; then
    pass "test script is $script_lines lines (≤ 500)"
else
    fail "test script is $script_lines lines (exceeds 500-line limit)"
fi

# -- summary ------------------------------------------------------------------
if [ "$failures" -gt 0 ]; then
    echo "migration-fixture regression FAILED (${failures} failure(s))"
    exit 1
fi
echo "migration-fixture regression PASSED"
