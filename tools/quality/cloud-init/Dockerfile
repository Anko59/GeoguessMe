# syntax=docker/dockerfile:1
# Test-only native parser: use the same Ubuntu release as the hosted server.
FROM hashicorp/terraform:1.16.5@sha256:c7926feace05d0f7e73542842bf3945924e955a1f782cf000ccbb8d18fa42d77 AS terraform
WORKDIR /opt/terraform-pins
# Download only the module's existing pinned providers during the image build;
# initialization never configures a provider or reads cloud state/credentials.
COPY infra/terraform/versions.tf infra/terraform/.terraform.lock.hcl ./
RUN CHECKPOINT_DISABLE=1 terraform init -backend=false -input=false

FROM ubuntu:24.04@sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55

# The official distro package supplies cloud-init and its native YAML parser.
# hadolint ignore=DL3008
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends cloud-init \
    && rm -rf /var/lib/apt/lists/*
COPY --from=terraform /bin/terraform /usr/local/bin/terraform
COPY --from=terraform /opt/terraform-pins/.terraform/providers /opt/terraform-providers
ENV PYTHONDONTWRITEBYTECODE=1 \
    TF_IN_AUTOMATION=1 \
    CHECKPOINT_DISABLE=1
WORKDIR /workspace
CMD ["python3", "tools/quality/cloud-init/test-user-data.py"]
