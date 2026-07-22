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
      { hostname = "deploy.${var.domain}", service = "ssh://127.0.0.1:22" },
      { service = "http_status:404" }
    ]
  }
}

data "cloudflare_zero_trust_tunnel_cloudflared_token" "app" {
  account_id = var.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.app.id
}

resource "cloudflare_dns_record" "tunnel" {
  for_each = toset(["@", "dev", "deploy"])
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

resource "cloudflare_zero_trust_access_identity_provider" "email_otp" {
  account_id = var.cloudflare_account_id
  name       = "Email one-time PIN"
  type       = "onetimepin"
  config     = {}
}

resource "cloudflare_zero_trust_access_application" "dev" {
  account_id                = var.cloudflare_account_id
  name                      = "GeoGuessMe dev"
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
    name       = "CI health service token"
    decision   = "non_identity"
    precedence = 2
    include = [{
      service_token = { token_id = cloudflare_zero_trust_access_service_token.github.id }
    }]
  }]
}

resource "cloudflare_zero_trust_access_service_token" "github" {
  account_id = var.cloudflare_account_id
  name       = "GeoGuessMe GitHub Actions"
  duration   = "8760h"
}

resource "cloudflare_zero_trust_access_application" "deployment" {
  account_id                = var.cloudflare_account_id
  name                      = "GeoGuessMe deployment SSH"
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
    name       = "GitHub Actions service token"
    decision   = "non_identity"
    precedence = 2
    include = [{
      service_token = { token_id = cloudflare_zero_trust_access_service_token.github.id }
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
    tunnel_token       = data.cloudflare_zero_trust_tunnel_cloudflared_token.app.token
    common_script      = base64encode(file("${path.module}/../../deployment/scripts/hosted/common.sh"))
    deploy_script      = base64encode(file("${path.module}/../../deployment/scripts/hosted/deploy.sh"))
    forced_script      = base64encode(file("${path.module}/../../deployment/scripts/hosted/forced-command.sh"))
    backup_script      = base64encode(file("${path.module}/../../deployment/scripts/hosted/backup.sh"))
    restore_script     = base64encode(file("${path.module}/../../deployment/scripts/hosted/restore-rehearsal.sh"))
    health_script      = base64encode(file("${path.module}/../../deployment/scripts/hosted/health-check.sh"))
    alert_script       = base64encode(file("${path.module}/../../deployment/scripts/hosted/alert.sh"))
    production_compose = base64encode(file("${path.module}/../../deployment/compose.production.yaml"))
    hosted_compose     = base64encode(file("${path.module}/../../deployment/compose.hosted.yaml"))
  })

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  lifecycle {
    prevent_destroy = true
  }
}
