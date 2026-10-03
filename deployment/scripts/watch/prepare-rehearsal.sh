#!/bin/sh
# Sourced by the watch rehearsal and its Dockerized fixture contracts.
prepare_watch_fixture() (
    root=$(CDPATH='' cd -- "$1" && pwd -P) || exit 1
    temporary=$2
    production_mock=$3
    development_mock=$4
    [ -d "$temporary" ] && [ ! -L "$temporary" ] || exit 1
    [ -d "$root/deployment/watch" ] && [ ! -L "$root/deployment/watch" ] || exit 1
    for name in Caddyfile vector.yaml victoria-metrics.yaml; do
        [ -f "$root/deployment/watch/$name" ] && [ ! -L "$root/deployment/watch/$name" ] || exit 1
    done
    mkdir -m 0755 "$temporary/public"
    for name in Caddyfile vector.yaml victoria-metrics.yaml; do
        cp "$root/deployment/watch/$name" "$temporary/public/$name"
        chmod 0644 "$temporary/public/$name"
    done
    # Collect only our two fake containers; retain the production label filter
    # so the negative development-labelled-container assertion stays meaningful.
    sed "/type: docker_logs/a\\
    include_containers: [\"$production_mock\", \"$development_mock\"]" \
        "$temporary/public/vector.yaml" >"$temporary/public/vector.rendered"
    mv "$temporary/public/vector.rendered" "$temporary/public/vector.yaml"
    chmod 0644 "$temporary/public/vector.yaml"
    printf 'KEY=replace-with-beszel-public-key\nTOKEN=replace-with-beszel-agent-token\n' >"$temporary/agent.env"
    chmod 0600 "$temporary/agent.env"
    : >"$temporary/production-metrics-token"
    printf ':8080 {\n\tlog\n\t@metrics_auth {\n\t\tpath /metrics\n\t\theader Authorization "Bearer rehearsal-metrics-token"\n\t}\n\t@metrics {\n\t\tpath /metrics\n\t}\n\thandle @metrics_auth {\n\t\troot * /srv\n\t\tfile_server\n\t}\n\thandle @metrics {\n\t\trespond 401\n\t}\n\trespond /health/ready 200\n\troot * /srv\n\tfile_server\n}\n' >"$temporary/mock.Caddyfile"
    printf 'up 1\n' >"$temporary/metrics"
    # These exact three files contain only generated fake fixture values. Never
    # normalize host secret directories or production token/environment files.
    chmod 0644 "$temporary/mock.Caddyfile" "$temporary/metrics" "$temporary/production-metrics-token"
    cat >"$temporary/override.yaml" <<YAML
services:
  gateway:
    volumes: !override
      - $temporary/public/Caddyfile:/etc/caddy/Caddyfile:ro
      - gateway_data:/data
      - gateway_config:/config
  victoria-metrics:
    volumes: !override
      - victoria_metrics_data:/victoria-metrics-data
      - $temporary/public/victoria-metrics.yaml:/etc/victoria-metrics/prometheus.yml:ro
      - $temporary:/run/watch-metrics:ro
  vector:
    volumes: !override
      - $temporary/public/vector.yaml:/etc/vector/vector.yaml:ro
      - vector_data:/var/lib/vector
  beszel-agent:
    env_file: !override
      - path: $temporary/agent.env
        required: true
YAML
)
