mock_provider "cloudflare" {
  alias = "mock"

  mock_data "cloudflare_zero_trust_tunnel_cloudflared_token" {
    defaults = {
      token = "mock-tunnel-token"
    }
  }
}
mock_provider "hcloud" {
  alias = "mock"

  # hcloud exposes resource IDs as strings, but firewall_ids requires numbers.
  mock_resource "hcloud_firewall" {
    defaults = {
      id = "123456"
    }
  }
}
mock_provider "random" {
  alias = "mock"
}

# Run only through the isolated, network-disabled Dockerized Make target.
# All three providers are explicit mocks; test state and inputs are disposable.
run "hosted_mocked_bootstrap" {
  command = apply
  providers = {
    cloudflare = cloudflare.mock
    hcloud     = hcloud.mock
    random     = random.mock
  }

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
    dev_access_emails            = ["developer@example.test"]
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
    condition = nonsensitive(
      length(base64encode(hcloud_server.app.user_data)) / 4 * 3 -
      length(regexall("=", base64encode(hcloud_server.app.user_data))) <= 32768
    )
    error_message = "Actual MIME cloud-init user_data must fit Hetzner's strict 32768 UTF-8 byte limit, including headers."
  }

  assert {
    condition = nonsensitive(hcloud_server.app.user_data == join("\n", concat(
      ["MIME-Version: 1.0", "Content-Type: application/gzip", "Content-Transfer-Encoding: base64", ""],
      regexall(".{1,76}", base64gzip(local.runtime_cloud_archive)),
      [""],
    )))
    error_message = "The actual server user_data must equal standard base64 MIME of the one gzip-compressed cloud-config archive."
  }

  assert {
    condition = nonsensitive(
      local.runtime_cloud_config_template == templatefile("../cloud-init/cloud-config.yaml.tftpl", {
        admin_key              = var.admin_ssh_public_key
        dev_ci_key             = var.dev_ci_ssh_public_key
        production_key         = var.production_ci_ssh_public_key
        runtime_revision       = var.runtime_revision
        tunnel_token           = "mock-tunnel-token"
        runtime_bundle         = local.runtime_bundle
        runtime_installer      = local.runtime_installer
        host_bootstrap         = local.host_bootstrap
        cloudflared_version    = local.host_tool_pins.cloudflared.version
        cloudflared_deb_sha256 = local.host_tool_pins.cloudflared.debSha256
      }) &&
      yamldecode(local.runtime_cloud_config) == yamldecode(local.runtime_cloud_config_template) &&
      local.runtime_cloud_archive == "#cloud-config-archive\n- type: text/cloud-config\n  content: |1\n   ${indent(3, chomp(local.runtime_cloud_config))}\n"
    )
    error_message = "The compact single-entry archive must preserve the complete source cloud-config, fake key/token inputs, and reviewed host-tool pins."
  }

  assert {
    condition = nonsensitive(
      length(local.runtime_bundle_files) == 33 &&
      alltrue([for content in local.runtime_bundle_files : endswith(content, "\n")]) &&
      length([for entry in yamldecode(local.runtime_cloud_config).write_files : entry if
        entry.path == "/tmp/geoguessme-runtime-bundle" &&
        entry.content == local.runtime_bundle && !can(entry.encoding)
      ]) == 1 &&
      length([for entry in yamldecode(local.runtime_cloud_config).write_files : entry if
        entry.path == "/usr/local/sbin/geoguessme-install-runtime-bundle" &&
        entry.content == local.runtime_installer && !can(entry.encoding)
      ]) == 1 &&
      length([for entry in yamldecode(local.runtime_cloud_config).write_files : entry if
        entry.path == "/usr/local/sbin/geoguessme-bootstrap-host" &&
        entry.content == local.host_bootstrap && !can(entry.encoding)
      ]) == 1
    )
    error_message = "Cloud-config must preserve all 33 framed UTF-8 members and the raw installer as exact literal contents, without nested compression."
  }

  assert {
    condition     = length(cloudflare_r2_bucket.media) == 2
    error_message = "Dev and production need separate media buckets."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.dev_health.domain == "dev.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.dev_health.policies) == 2 &&
      length([
        for p in cloudflare_zero_trust_access_application.dev_health.policies : p
        if p.name == "Owner email OTP" &&
        contains([for inc in p.include : try(inc.email.email, "")], "developer@example.test")
      ]) == 1
    )
    error_message = "Dev Access must allow approved human identities and its service token."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.dev_deployment.domain == "deploy.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.dev_deployment.policies) == 2
    )
    error_message = "Dev deployment Access must allow its owner and service token."
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
      cloudflare_zero_trust_access_application.watch.domain == "watch.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.watch.allowed_idps) == 1 &&
      cloudflare_zero_trust_access_identity_provider.email_otp.type == "onetimepin" &&
      cloudflare_zero_trust_access_application.watch.auto_redirect_to_identity &&
      length(cloudflare_zero_trust_access_application.watch.policies) == 1 &&
      cloudflare_zero_trust_access_application.watch.policies[0].decision == "allow" &&
      contains([for inc in cloudflare_zero_trust_access_application.watch.policies[0].include : try(inc.email.email, "")], "jeancollette138@gmail.com")
    )
    error_message = "Monitoring Access must allow only the owner email through one policy."
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
      cloudflare_dns_record.tunnel["watch"].name == "watch" &&
      strcontains(jsonencode(cloudflare_zero_trust_tunnel_cloudflared_config.app.config), "http://127.0.0.1:8083") &&
      strcontains(jsonencode(cloudflare_zero_trust_tunnel_cloudflared_config.app.config), "http://127.0.0.1:8084") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "00-geoguessme.conf") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "PasswordAuthentication no") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "d /run/lock/geoguessme 0750 deploy deploy -") &&
      strcontains(file("../cloud-init/bootstrap-host.sh"), "systemd-tmpfiles --create /etc/tmpfiles.d/geoguessme.conf") &&
      length(regexall("defer: true", file("../cloud-init/cloud-config.yaml.tftpl"))) == 2 &&
      strcontains(file("../cloud-init/bootstrap-host.sh"), "ufw allow in on lo to any") &&
      strcontains(file("../cloud-init/bootstrap-host.sh"), "chown -R deploy:deploy /etc/geoguessme/age") &&
      strcontains(file("../cloud-init/bootstrap-host.sh"), "geoguessme-backup@dev.timer") &&
      strcontains(file("../cloud-init/install-runtime-bundle.sh"), "geoguessme-watch-health.timer") &&
      strcontains(file("../cloud-init/install-runtime-bundle.sh"), "watch-refresh-metrics-token") &&
      strcontains(file("../cloud-init/bootstrap-host.sh"), "systemctl enable --now") &&
      strcontains(file("../cloud-init/install-runtime-bundle.sh"), "/opt/geoguessme/config/compose.production.yaml") &&
      strcontains(file("../cloud-init/install-runtime-bundle.sh"), "/opt/geoguessme/config/compose.watch.yaml")
    )
    error_message = "Cloud-init must secure SSH, recreate volatile locks, and schedule backups."
  }
}
