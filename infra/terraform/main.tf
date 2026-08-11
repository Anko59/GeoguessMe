// Preserve existing Access resources during the policy split. The legacy
// service token remains live but leaves Terraform state so operators can revoke
// it only after all three replacement credentials have been verified.
moved {
  from = cloudflare_zero_trust_access_application.dev
  to   = cloudflare_zero_trust_access_application.dev_health
}

moved {
  from = cloudflare_zero_trust_access_application.deployment
  to   = cloudflare_zero_trust_access_application.dev_deployment
}

removed {
  from = cloudflare_zero_trust_access_service_token.github

  lifecycle {
    destroy = false
  }
}

locals {
  # Hashes are rendered into a root-owned host manifest during provisioning.
  # The runtime verifier must not use the deploy-writable release archive as
  # its expected baseline because an attacker with deploy access could alter
  # both the installed file and the comparison source.
  runtime_hash_files = {
    "bin/alert.sh"                    = "${path.module}/../../deployment/scripts/hosted/alert.sh"
    "bin/backup.sh"                   = "${path.module}/../../deployment/scripts/hosted/backup.sh"
    "bin/common.sh"                   = "${path.module}/../../deployment/scripts/hosted/common.sh"
    "bin/deploy.sh"                   = "${path.module}/../../deployment/scripts/hosted/deploy.sh"
    "bin/forced-command.sh"           = "${path.module}/../../deployment/scripts/hosted/forced-command.sh"
    "bin/health-check.sh"             = "${path.module}/../../deployment/scripts/hosted/health-check.sh"
    "bin/restore-rehearsal.sh"        = "${path.module}/../../deployment/scripts/hosted/restore-rehearsal.sh"
    "bin/verify-deployment-hashes.sh" = "${path.module}/../../deployment/scripts/hosted/verify-deployment-hashes.sh"
    "config/compose.hosted.yaml"      = "${path.module}/../../deployment/compose.hosted.yaml"
    "config/compose.production.yaml"  = "${path.module}/../../deployment/compose.production.yaml"
  }

  runtime_hashes = join("\n", [
    for relative_path, absolute_path in local.runtime_hash_files :
    "${filesha256(absolute_path)}  ${relative_path}"
  ])
}

resource "random_bytes" "tunnel_secret" {
  length = 32
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "app" {
  account_id    = var.cloudflare_account_id
  name          = "geoguessme"
  config_src    = "cloudflare"
  tunnel_secret = random_bytes.tunnel_secret.base64
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "app" {
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.app.id
  source     = "cloudflare"
  config = {
    ingress = [
      { hostname = var.domain, service = "http://127.0.0.1:8081" },
      { hostname = "dev.${var.domain}", service = "http://127.0.0.1:8082" },
      # Development CI reaches SSH through deploy.geoguessme.com and production
      # CI through deploy-prod.geoguessme.com; the Access application on each
      # hostname accepts only its own service token (F-05 isolation).
      { hostname = "deploy.${var.domain}", service = "ssh://127.0.0.1:22" },
      { hostname = "deploy-prod.${var.domain}", service = "ssh://127.0.0.1:22" },
      { service = "http_status:404" }
    ]
  }
}

data "cloudflare_zero_trust_tunnel_cloudflared_token" "app" {
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.app.id
}

resource "cloudflare_dns_record" "tunnel" {
  for_each = toset(["@", "dev", "deploy", "deploy-prod"])
  zone_id  = var.cloudflare_zone_id
  name     = each.value
  type     = "CNAME"
  content  = "${cloudflare_zero_trust_tunnel_cloudflared.app.id}.cfargotunnel.com"
  proxied  = true
  ttl      = 1
}

resource "cloudflare_dns_record" "brevo" {
  for_each = var.brevo_dns_records
  zone_id  = var.cloudflare_zone_id
  name     = each.value.name
  type     = each.value.type
  content  = each.value.content
  proxied  = false
  ttl      = 3600
}

resource "cloudflare_dns_record" "dmarc" {
  zone_id = var.cloudflare_zone_id
  name    = "_dmarc"
  type    = "TXT"
  content = "v=DMARC1; p=none; rua=mailto:dmarc@${var.domain}; adkim=s; aspf=s"
  proxied = false
  ttl     = 3600
}

# Single combined SPF record: authorizes Brevo transactional mail plus the
# Cloudflare Email Routing forwarding infrastructure. RFC 7208 forbids multiple
# SPF records, so brevo_dns_records must never contain another v=spf1 TXT.
resource "cloudflare_dns_record" "spf" {
  zone_id = var.cloudflare_zone_id
  name    = "@"
  type    = "TXT"
  content = var.spf_record
  proxied = false
  ttl     = 3600
}

# F-02: one authoritative Cloudflare edge policy. HSTS starts at max-age=86400
# without subdomains; raise to 15552000 with includeSubDomains after seven green
# days (see docs/runbooks/hsts-rollout.md). Preload stays off for this program.
resource "cloudflare_zone_setting" "always_use_https" {
  zone_id    = var.cloudflare_zone_id
  setting_id = "always_use_https"
  value      = "on"
}

resource "cloudflare_zone_setting" "min_tls_version" {
  zone_id    = var.cloudflare_zone_id
  setting_id = "min_tls_version"
  value      = "1.2"
}

resource "cloudflare_zone_setting" "tls_1_3" {
  zone_id    = var.cloudflare_zone_id
  setting_id = "tls_1_3"
  value      = "on"
}

resource "cloudflare_zone_setting" "security_header" {
  zone_id    = var.cloudflare_zone_id
  setting_id = "security_header"
  value = {
    strict_transport_security = {
      enabled            = true
      include_subdomains = false
      max_age            = 86400
      nosniff            = true
      preload            = false
    }
  }
}

# F-11: Cloudflare Email Routing forwards dmarc@geoguessme.com (the DMARC rua
# mailbox) to the operator mailbox. Zone activation and address verification are
# live steps; the routing DNS records (MX, forwarding) are published by
# Cloudflare once email routing is enabled.
resource "cloudflare_email_routing_settings" "main" {
  zone_id = var.cloudflare_zone_id
}

resource "cloudflare_email_routing_address" "operator" {
  account_id = var.cloudflare_account_id
  email      = var.operator_email
}

resource "cloudflare_email_routing_rule" "dmarc" {
  count = var.enable_dmarc_forwarding ? 1 : 0

  zone_id  = var.cloudflare_zone_id
  name     = "Forward DMARC reports to operator mailbox"
  enabled  = true
  priority = 0
  matchers = [{
    type  = "literal"
    field = "to"
    value = "dmarc@${var.domain}"
  }]
  actions = [{
    type  = "forward"
    value = [var.operator_email]
  }]

  depends_on = [
    cloudflare_email_routing_address.operator,
    cloudflare_email_routing_settings.main,
  ]
}

resource "cloudflare_zero_trust_access_identity_provider" "email_otp" {
  account_id = var.cloudflare_account_id
  name       = "Email one-time PIN"
  type       = "onetimepin"
  config     = {}
}

resource "cloudflare_zero_trust_access_application" "dev_health" {
  account_id                = var.cloudflare_account_id
  name                      = "GeoGuessMe dev health"
  domain                    = "dev.${var.domain}"
  type                      = "self_hosted"
  session_duration          = "24h"
  allowed_idps              = [cloudflare_zero_trust_access_identity_provider.email_otp.id]
  auto_redirect_to_identity = true
  policies = [{
    name       = "Owner email OTP"
    decision   = "allow"
    precedence = 1
    include    = [{ email = { email = var.access_email } }]
    }, {
    name       = "Development health service token"
    decision   = "non_identity"
    precedence = 2
    include = [{
      service_token = { token_id = var.dev_health_token_id }
    }]
  }]
}

resource "cloudflare_zero_trust_access_application" "dev_deployment" {
  account_id                = var.cloudflare_account_id
  name                      = "GeoGuessMe dev deployment SSH"
  domain                    = "deploy.${var.domain}"
  type                      = "self_hosted"
  session_duration          = "1h"
  allowed_idps              = [cloudflare_zero_trust_access_identity_provider.email_otp.id]
  auto_redirect_to_identity = true
  policies = [{
    name       = "Owner operator access"
    decision   = "allow"
    precedence = 1
    include    = [{ email = { email = var.access_email } }]
    }, {
    name       = "Development deployment service token"
    decision   = "non_identity"
    precedence = 2
    include = [{
      service_token = { token_id = var.dev_deploy_token_id }
    }]
  }]
}

resource "cloudflare_zero_trust_access_application" "prod_deployment" {
  account_id                = var.cloudflare_account_id
  name                      = "GeoGuessMe production deployment SSH"
  domain                    = "deploy-prod.${var.domain}"
  type                      = "self_hosted"
  session_duration          = "1h"
  allowed_idps              = [cloudflare_zero_trust_access_identity_provider.email_otp.id]
  auto_redirect_to_identity = true
  policies = [{
    name       = "Owner operator access"
    decision   = "allow"
    precedence = 1
    include    = [{ email = { email = var.access_email } }]
    }, {
    name       = "Production deployment service token"
    decision   = "non_identity"
    precedence = 2
    include = [{
      service_token = { token_id = var.prod_deploy_token_id }
    }]
  }]
}

resource "cloudflare_r2_bucket" "media" {
  for_each      = toset(["geoguessme-dev-media", "geoguessme-production-media"])
  account_id    = var.cloudflare_account_id
  name          = each.value
  location      = "weur"
  storage_class = "Standard"
}

resource "cloudflare_r2_bucket" "database_backups" {
  account_id    = var.cloudflare_account_id
  name          = "geoguessme-database-backups"
  location      = "weur"
  storage_class = "Standard"
}

resource "hcloud_firewall" "deny_inbound" {
  name = "geoguessme-deny-inbound"
}

resource "hcloud_server" "app" {
  name               = "geoguessme"
  image              = "ubuntu-24.04"
  server_type        = "cx23"
  location           = "nbg1"
  backups            = true
  delete_protection  = true
  rebuild_protection = true
  firewall_ids       = [hcloud_firewall.deny_inbound.id]
  user_data = templatefile("${path.module}/../cloud-init/cloud-config.yaml.tftpl", {
    admin_key          = var.admin_ssh_public_key
    dev_ci_key         = var.dev_ci_ssh_public_key
    production_key     = var.production_ci_ssh_public_key
    runtime_revision   = var.runtime_revision
    tunnel_token       = data.cloudflare_zero_trust_tunnel_cloudflared_token.app.token
    common_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/common.sh"))
    deploy_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/deploy.sh"))
    forced_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/forced-command.sh"))
    verify_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/verify-deployment-hashes.sh"))
    backup_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/backup.sh"))
    restore_script     = base64gzip(file("${path.module}/../../deployment/scripts/hosted/restore-rehearsal.sh"))
    health_script      = base64gzip(file("${path.module}/../../deployment/scripts/hosted/health-check.sh"))
    alert_script       = base64gzip(file("${path.module}/../../deployment/scripts/hosted/alert.sh"))
    production_compose = base64gzip(file("${path.module}/../../deployment/compose.production.yaml"))
    hosted_compose     = base64gzip(file("${path.module}/../../deployment/compose.hosted.yaml"))
    runtime_hashes     = local.runtime_hashes
  })

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  lifecycle {
    prevent_destroy = true
    ignore_changes  = [user_data]
  }
}
