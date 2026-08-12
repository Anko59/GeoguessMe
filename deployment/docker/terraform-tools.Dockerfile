# syntax=docker/dockerfile:1
# Validation-only Terraform image with the fixed x/net dependency used by the
# pinned Terraform release.
FROM golang:1.25.12-alpine@sha256:56961d79ea8129efddcc0b8643fd8a5416b4e6228cfd477e3fd61deb2672c587 AS terraform-build
RUN apk add --no-cache git=2.54.0-r0
WORKDIR /src
RUN git clone --depth 1 --branch v1.15.8 https://github.com/hashicorp/terraform.git /src \
    && test "$(git rev-parse HEAD)" = b9e178decf87d274d25ed36bc5a4dbc857e00420 \
    && go mod edit -replace=golang.org/x/net@v0.55.0=golang.org/x/net@v0.56.0 \
    && go mod download golang.org/x/net \
    && go mod tidy \
    && go build -trimpath \
        -ldflags='-s -w -X github.com/hashicorp/terraform/version.dev=no' \
        -o /out/terraform .

FROM hashicorp/terraform:1.15.8@sha256:7ae513256f7ce67879e218ae8593d6fbe216ec9e123abe6c94e4e10704857963
COPY --from=terraform-build /out/terraform /bin/terraform
