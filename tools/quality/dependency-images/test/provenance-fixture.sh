#!/usr/bin/env bash
# BuildKit v0.33 SLSA v1 fields follow docs/attestations/slsa-definitions.md.
# Only fake imagetools output; no daemon, registry or credentials are consulted.
set -euo pipefail
mode=${FAKE_PROVENANCE:-legacy}
case "$mode" in
    missing-slsa)
        printf '{}\n'
        exit
        ;;
    malformed-json)
        printf '{invalid-json\n'
        exit
        ;;
esac
jq -n --arg inputs "${1:?}" --arg base "${2:?}" --arg mode "$mode" \
    --arg legacy 'https://mobyproject.org/buildkit@v1' \
    --arg modern 'https://github.com/moby/buildkit/blob/master/docs/attestations/slsa-definitions.md' '
    {"linux/amd64": {SLSA: {
        buildDefinition: {
            buildType: $modern,
            externalParameters: {
                configSource: {path: "Dockerfile"},
                request: {frontend: "dockerfile.v0", args: {"build-arg:DEPENDENCY_INPUTS": $inputs},
                    locals: [{name: "context"}, {name: "dockerfile"}], secrets: [], ssh: []}
            },
            internalParameters: {builderPlatform: "linux/amd64", buildConfig: {llbDefinition: []}},
            resolvedDependencies: [{uri: "pkg:docker/ghcr.io/getsops/sops@v3.13.3?platform=linux%2Famd64",
                digest: {sha256: ($base | ltrimstr("sha256:"))}}]
        },
        runDetails: {
            builder: {id: "https://github.com/Anko59/GeoguessMe/actions/runs/37242418190"},
            metadata: {invocationID: "fixture-buildkit-v033", startedOn: "2026-10-04T22:00:00Z",
                finishedOn: "2026-10-04T22:01:00Z", buildkit_hermetic: false,
                buildkit_completeness: {request: true, resolvedDependencies: false}, buildkit_reproducible: false}
        }
    }}} |
    if $mode == "legacy" then .["linux/amd64"].SLSA = {buildType: $legacy}
    elif $mode == "legacy-nested" then .["linux/amd64"].SLSA.buildDefinition.buildType = $legacy
    elif $mode == "direct-legacy" then .["linux/amd64"].SLSA = {buildType: $legacy}
    elif (["modern", "mapped-modern", "direct-multi-runtime", "ambiguous", "ambiguous-null", "root-null", "root-string", "mapped-null", "mapped-nonobject", "wrong-arch", "slsa1", "arbitrary-root", "direct-extra"] | index($mode)) != null then .
    elif $mode == "unknown-url" then .["linux/amd64"].SLSA.buildDefinition.buildType = "https://example.invalid/buildkit"
    elif $mode == "modern-suffix" then .["linux/amd64"].SLSA.buildDefinition.buildType += "?unreviewed=1"
    elif $mode == "root-modern" then .["linux/amd64"].SLSA = {buildType: $modern}
    elif $mode == "empty" then .["linux/amd64"].SLSA.buildDefinition.buildType = ""
    elif $mode == "malformed" then .["linux/amd64"].SLSA.buildDefinition = $modern
    elif $mode == "definition-array" then .["linux/amd64"].SLSA.buildDefinition = [{buildType: $modern}]
    elif $mode == "slsa-array" then .["linux/amd64"].SLSA = [{buildDefinition: {buildType: $modern}}]
    elif $mode == "type-array" then .["linux/amd64"].SLSA.buildDefinition.buildType = [$modern]
    elif $mode == "missing-definition" then del(.["linux/amd64"].SLSA.buildDefinition)
    else error("unsupported fake provenance case: " + $mode)
    end |
    if (["legacy", "legacy-nested", "mapped-modern"] | index($mode)) != null then .
    elif $mode == "ambiguous" then . + {SLSA: .["linux/amd64"].SLSA}
    elif $mode == "ambiguous-null" then . + {SLSA: null}
    elif $mode == "root-null" then {SLSA: null}
    elif $mode == "root-string" then {SLSA: "invalid"}
    elif $mode == "mapped-null" then {"linux/amd64": {SLSA: null}}
    elif $mode == "mapped-nonobject" then {"linux/amd64": []}
    elif $mode == "wrong-arch" then {"linux/arm64": .["linux/amd64"]}
    elif $mode == "slsa1" then {SLSA1: .["linux/amd64"].SLSA}
    elif $mode == "arbitrary-root" then .["linux/amd64"].SLSA
    elif $mode == "direct-extra" then .["linux/amd64"] + {unexpected: true}
    else .["linux/amd64"] end'
