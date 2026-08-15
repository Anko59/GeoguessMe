mock_provider "cloudflare" {
  override_during = plan

  mock_data "cloudflare_zero_trust_tunnel_cloudflared_token" {
    defaults = {
      token = "mock-tunnel-token"
    }
  }
}
mock_provider "hcloud" {}
mock_provider "random" {}

run "hosted_plan" {
  command = plan

  variables {
    cloudflare_account_id        = "00000000000000000000000000000000"
    cloudflare_zone_id           = "11111111111111111111111111111111"
    admin_ssh_public_key         = "ssh-ed25519 AAAAoperator operator"
    dev_ci_ssh_public_key        = "ssh-ed25519 AAAAdevelopment development"
    production_ci_ssh_public_key = "ssh-ed25519 AAAAproduction production"
    runtime_revision             = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    dev_health_token_id          = "11111111-1111-4111-8111-111111111111"
    dev_deploy_token_id          = "22222222-2222-4222-8222-222222222222"
    prod_deploy_token_id         = "33333333-3333-4333-8333-333333333333"
    enable_dmarc_forwarding      = true
  }

  assert {
    condition = (
      hcloud_server.app.server_type == "cx23" &&
      hcloud_server.app.location == "nbg1" && hcloud_server.app.backups
    )
    error_message = "The shared host must remain the backed-up CX23 in Nuremberg."
  }

  assert {
    condition = (
      cloudflare_zone_setting.always_use_https.setting_id == "always_use_https" &&
      cloudflare_zone_setting.always_use_https.value == "on"
    )
    error_message = "Always Use HTTPS must be enforced at the edge."
  }

  assert {
    condition = (
      cloudflare_zone_setting.min_tls_version.setting_id == "min_tls_version" &&
      cloudflare_zone_setting.min_tls_version.value == "1.2"
    )
    error_message = "Minimum TLS version must be 1.2."
  }

  assert {
    condition = (
      cloudflare_zone_setting.security_header.value.strict_transport_security.enabled &&
      cloudflare_zone_setting.security_header.value.strict_transport_security.max_age == 86400 &&
      !cloudflare_zone_setting.security_header.value.strict_transport_security.preload
    )
    error_message = "HSTS must be enabled at max-age=86400 without preload during the initial rollout."
  }

  assert {
    condition = (
      cloudflare_email_routing_settings.main.zone_id == var.cloudflare_zone_id &&
      cloudflare_email_routing_rule.dmarc[0].matchers[0].value == "dmarc@geoguessme.com" &&
      cloudflare_email_routing_rule.dmarc[0].actions[0].value[0] == var.operator_email
    )
    error_message = "Email routing must forward DMARC reports from dmarc@geoguessme.com to the operator mailbox."
  }

  assert {
    condition = (
      cloudflare_dns_record.spf.name == "@" &&
      cloudflare_dns_record.spf.type == "TXT" &&
      strcontains(cloudflare_dns_record.spf.content, "_spf.mx.cloudflare.net") &&
      strcontains(cloudflare_dns_record.spf.content, "brevo")
    )
    error_message = "A single combined SPF record must authorize Brevo and Cloudflare email routing."
  }

  assert {
    condition = (
      hcloud_server.app.delete_protection &&
      hcloud_server.app.rebuild_protection
    )
    error_message = "Server delete and rebuild protection must remain enabled."
  }

  assert {
    condition = length(templatefile("../cloud-init/cloud-config.yaml.tftpl", {
      admin_key          = var.admin_ssh_public_key
      dev_ci_key         = var.dev_ci_ssh_public_key
      production_key     = var.production_ci_ssh_public_key
      runtime_revision   = var.runtime_revision
      tunnel_token       = "mock-tunnel-token"
      common_script      = base64gzip(file("../../deployment/scripts/hosted/common.sh"))
      deploy_script      = base64gzip(file("../../deployment/scripts/hosted/deploy.sh"))
      forced_script      = base64gzip(file("../../deployment/scripts/hosted/forced-command.sh"))
      verify_script      = base64gzip(file("../../deployment/scripts/hosted/verify-deployment-hashes.sh"))
      backup_script      = base64gzip(file("../../deployment/scripts/hosted/backup.sh"))
      restore_script     = base64gzip(file("../../deployment/scripts/hosted/restore-rehearsal.sh"))
      health_script      = base64gzip(file("../../deployment/scripts/hosted/health-check.sh"))
      alert_script       = base64gzip(file("../../deployment/scripts/hosted/alert.sh"))
      production_compose = base64gzip(file("../../deployment/compose.production.yaml"))
      hosted_compose     = base64gzip(file("../../deployment/compose.hosted.yaml"))
      runtime_hashes = join("\n", [
        for relative_path, absolute_path in {
          "bin/alert.sh"                    = "../../deployment/scripts/hosted/alert.sh"
          "bin/backup.sh"                   = "../../deployment/scripts/hosted/backup.sh"
          "bin/common.sh"                   = "../../deployment/scripts/hosted/common.sh"
          "bin/deploy.sh"                   = "../../deployment/scripts/hosted/deploy.sh"
          "bin/forced-command.sh"           = "../../deployment/scripts/hosted/forced-command.sh"
          "bin/health-check.sh"             = "../../deployment/scripts/hosted/health-check.sh"
          "bin/restore-rehearsal.sh"        = "../../deployment/scripts/hosted/restore-rehearsal.sh"
          "bin/verify-deployment-hashes.sh" = "../../deployment/scripts/hosted/verify-deployment-hashes.sh"
          "config/compose.hosted.yaml"      = "../../deployment/compose.hosted.yaml"
          "config/compose.production.yaml"  = "../../deployment/compose.production.yaml"
        } : "${filesha256(absolute_path)}  ${relative_path}"
      ])
    })) <= 32768
    error_message = "Rendered cloud-init must fit Hetzner's 32 KiB user-data limit."
  }

  assert {
    condition     = length(cloudflare_r2_bucket.media) == 2
    error_message = "Dev and production need separate media buckets."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.dev_health.domain == "dev.geoguessme.com"
    )
    error_message = "Access must protect the development hostname."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.dev_deployment.domain == "deploy.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.dev_deployment.policies) == 2
    )
    error_message = "Dev deployment Access must allow both the owner and the dev deployment service token."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.prod_deployment.domain == "deploy-prod.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.prod_deployment.policies) == 2
    )
    error_message = "Production deployment Access must allow both the owner and the production deployment service token."
  }

  assert {
    condition = (
      length([for p in cloudflare_zero_trust_access_application.dev_health.policies : p if length([for inc in p.include : try(inc.service_token.token_id, "") if try(inc.service_token.token_id, "") == var.dev_health_token_id]) > 0]) == 1 &&
      length([for p in cloudflare_zero_trust_access_application.dev_deployment.policies : p if length([for inc in p.include : try(inc.service_token.token_id, "") if try(inc.service_token.token_id, "") == var.dev_deploy_token_id]) > 0]) == 1 &&
      length([for p in cloudflare_zero_trust_access_application.prod_deployment.policies : p if length([for inc in p.include : try(inc.service_token.token_id, "") if try(inc.service_token.token_id, "") == var.prod_deploy_token_id]) > 0]) == 1
    )
    error_message = "Each Access application must reference its own externally created service-token ID."
  }

  assert {
    condition = (
      var.domain == "geoguessme.com" &&
      cloudflare_dns_record.tunnel["auth"].name == "auth" &&
      strcontains(jsonencode(cloudflare_zero_trust_tunnel_cloudflared_config.app.config), "http://127.0.0.1:8083") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "00-geoguessme.conf") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "PasswordAuthentication no") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "d /run/lock/geoguessme 0750 deploy deploy -") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "[systemd-tmpfiles, --create, /etc/tmpfiles.d/geoguessme.conf]") &&
      length(regexall("defer: true", file("../cloud-init/cloud-config.yaml.tftpl"))) == 2 &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "[ufw, allow, in, \"on\", lo, to, any]") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "[chown, -R, deploy:deploy, /etc/geoguessme/age]") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "geoguessme-backup@dev.timer") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "systemctl, enable, --now") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "/opt/geoguessme/config/compose.production.yaml")
    )
    error_message = "Cloud-init must secure SSH, recreate volatile locks, and schedule backups."
  }
}
