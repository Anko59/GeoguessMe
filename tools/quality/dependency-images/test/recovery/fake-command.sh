#!/usr/bin/env bash
# All external operations in recovery tests are inert, recorded fakes.
set -euo pipefail
name=${0##*/}
printf '%s %s\n' "$name" "$*" >>"$TEST_STATE/trace"
index=sha256:f3b02924f109607058d1238eab1c06ec7fdc9f6151beb5252ec765f3fe5060f3
runtime=sha256:41e201ab66e028e3130f18b3c0073564846d5aeeff40c7a6ba3462dc0d708377
inputs=cb1dd9163b630bbda840ba2906310e4b402f55e69bc81b4bc4c8f95c26511689
revision=a228769dbaf98ce8a5aff7fb0946456f34f8e21b
case "$name" in
    sleep) exit 0 ;;
    gh)
        case "$*" in
            *runs/37242418190)
                [[ "$TEST_CASE" != api-denied ]] || {
                    echo 'HTTP 403 forbidden' >&2
                    exit 1
                }
                jq -n --arg revision "$revision" '{id:37242418190,head_sha:$revision,head_branch:"dev",event:"push",path:".github/workflows/security.yml",repository:{full_name:"Anko59/GeoguessMe"},conclusion:"failure"}' >"$TEST_STATE/run.json"
                case "$TEST_CASE" in
                    run-origin) jq '.head_sha="wrong"' "$TEST_STATE/run.json" ;;
                    run-workflow) jq '.path=".github/workflows/other.yml"' "$TEST_STATE/run.json" ;;
                    run-branch) jq '.head_branch="main"' "$TEST_STATE/run.json" ;;
                    *) jq '.' "$TEST_STATE/run.json" ;;
                esac
                ;;
            *jobs/111553605968/logs)
                [[ " $* " == *' --allow-escape-sequences '* ]] || {
                    echo 'the response contains terminal escape sequences; pass --allow-escape-sequences to output it anyway' >&2
                    exit 1
                }
                [[ "$TEST_CASE" != log-denied ]] || {
                    echo 'HTTP 403 forbidden' >&2
                    exit 1
                }
                [[ "$TEST_CASE" != log-digest ]] || index=sha256:wrong
                printf '\033[32mpushing manifest for ghcr.io/anko59/geoguessme-keycloak:dependency-%s@%s\033[0m\n' "$inputs" "$index"
                [[ "$TEST_CASE" == log-failure ]] || echo 'dependency-images: missing BuildKit SLSA provenance: keycloak'
                ;;
            *jobs/111553605968)
                jq -n '{id:111553605968,run_id:37242418190,name:"audit-images",conclusion:"failure",steps:[{name:"Explicit dependency preparation (only changed inputs build)",conclusion:"failure"}]}' >"$TEST_STATE/job.json"
                case "$TEST_CASE" in
                    job-origin) jq '.run_id=0' "$TEST_STATE/job.json" ;;
                    job-failure) jq '.steps[0].conclusion="success"' "$TEST_STATE/job.json" ;;
                    *) jq '.' "$TEST_STATE/job.json" ;;
                esac
                ;;
            *) exit 2 ;;
        esac
        ;;
    docker)
        case "$*" in
            *imagetools*Manifest.Digest*)
                [[ "$TEST_CASE" != registry-denied ]] || {
                    echo 'HTTP 403 forbidden' >&2
                    exit 1
                }
                [[ "$TEST_CASE" != registry-missing ]] || {
                    echo 'MANIFEST_UNKNOWN' >&2
                    exit 1
                }
                [[ "$TEST_CASE" != tag-digest ]] || index=sha256:wrong
                if [[ "$TEST_CASE" == tag-race && -f "$TEST_STATE/audited" ]]; then index=sha256:wrong; fi
                jq -n --arg index "$index" '$index'
                ;;
            *imagetools*--raw*)
                [[ "$TEST_CASE" != runtime-digest ]] || runtime=sha256:wrong
                jq -n --arg runtime "$runtime" '{manifests:[{digest:$runtime,platform:{os:"linux",architecture:"amd64"}},{digest:"attestation",platform:{os:"unknown",architecture:"unknown"},annotations:{"vnd.docker.reference.type":"attestation-manifest","vnd.docker.reference.digest":$runtime}}]}' >"$TEST_STATE/index.json"
                case "$TEST_CASE" in
                    attestation-link) jq '.manifests[1].annotations["vnd.docker.reference.digest"]="wrong"' "$TEST_STATE/index.json" ;;
                    multiple-runtime) jq '.manifests += [.manifests[0]]' "$TEST_STATE/index.json" ;;
                    direct-multi-runtime) jq '.manifests += [{digest:"other-runtime",platform:{os:"linux",architecture:"arm64"}}]' "$TEST_STATE/index.json" ;;
                    *) jq '.' "$TEST_STATE/index.json" ;;
                esac
                ;;
            *imagetools*Provenance*) jq "${TEST_PROVENANCE_FILTER:-.}" "$TEST_FIXTURE/producer.json" ;;
            'pull --platform linux/amd64 '*) exit 0 ;;
            'image inspect '*RepoDigests*)
                [[ "$TEST_CASE" != repo-digest ]] || index=sha256:wrong
                jq -n --arg index "$index" '["ghcr.io/anko59/geoguessme-keycloak@"+$index]'
                ;;
            'image inspect '*)
                [[ "$TEST_CASE" != local-inputs ]] || inputs=wrong
                architecture=amd64
                [[ "$TEST_CASE" != local-platform ]] || architecture=arm64
                base=sha256:37dbaf6f0722c9ec246335f36e1ef8b2e6cb960f7c27e0d8c615121a3d475a85
                [[ "$TEST_CASE" != local-base ]] || base=sha256:wrong
                jq -n --arg inputs "$inputs" --arg architecture "$architecture" --arg base "$base" --arg index "$index" '[{Id:$index,Os:"linux",Architecture:$architecture,Config:{Labels:{"dev.geoguessme.dependency-inputs":$inputs,"org.opencontainers.image.base.name":"quay.io/keycloak/keycloak:26.7.5","org.opencontainers.image.base.digest":$base}}}]'
                ;;
            *)
                echo 'unexpected Docker mutation' >&2
                exit 2
                ;;
        esac
        ;;
    make)
        [[ "$*" == *"audit-image-set IMAGE_AUDIT_REFS=ghcr.io/anko59/geoguessme-keycloak:dependency-$inputs@$index" ]] || exit 2
        [[ "$TEST_CASE" != audit-failed ]] || {
            echo 'vulnerability gate failed' >&2
            exit 42
        }
        touch "$TEST_STATE/audited"
        ;;
    cosign)
        [[ "$*" == *"dependency-inputs=$inputs"* && "$*" == *dependency-build=true* && "$*" == *"@$index" ]] || exit 2
        case "$1" in
            verify)
                [[ "$*" == *'https://token.actions.githubusercontent.com'* && "$*" == *'workflows/(deploy|security)'* ]] || exit 2
                if [[ -f "$TEST_STATE/signed" ]]; then
                    [[ "$TEST_CASE" != final-verify ]] || {
                        echo 'signature verification failed' >&2
                        exit 1
                    }
                    echo 'verified'
                    exit 0
                fi
                case "$TEST_CASE" in
                    invalid-signature) echo 'no matching signatures' >&2 ;;
                    signature-denied) echo 'HTTP 403 forbidden: no signatures found' >&2 ;;
                    signature-network) echo 'i/o timeout' >&2 ;;
                    signature-mixed) printf 'no signatures found\nconnection reset by peer\n' >&2 ;;
                    signature-empty) : ;;
                    signature-wrapper) printf 'Error: no signatures found\nerror during command execution: no signatures found\n' >&2 ;;
                    *) echo 'no signatures found' >&2 ;;
                esac
                exit 1
                ;;
            sign)
                [[ -f "$TEST_STATE/audited" ]] || exit 2
                [[ "$TEST_CASE" != sign-failed ]] || {
                    echo 'signing failed' >&2
                    exit 1
                }
                touch "$TEST_STATE/signed"
                ;;
            *) exit 2 ;;
        esac
        ;;
    *) exit 2 ;;
esac
