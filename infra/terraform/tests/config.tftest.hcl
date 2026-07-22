mock_provider "cloudflare" {
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
      hcloud_server.app.delete_protection &&
      hcloud_server.app.rebuild_protection
    )
    error_message = "Server delete and rebuild protection must remain enabled."
  }

  assert {
    condition     = length(cloudflare_r2_bucket.media) == 2
    error_message = "Dev and production need separate media buckets."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.dev.domain == "dev.geoguessme.com"
    )
    error_message = "Access must protect the development hostname."
  }

  assert {
    condition = (
      cloudflare_zero_trust_access_application.deployment.domain == "deploy.geoguessme.com" &&
      length(cloudflare_zero_trust_access_application.deployment.policies) == 2
    )
    error_message = "Deployment Access must allow both the owner and the CI service token."
  }

  assert {
    condition = (
      var.domain == "geoguessme.com" &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "PasswordAuthentication no") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "geoguessme-backup@dev.timer") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "systemctl, enable, --now") &&
      strcontains(file("../cloud-init/cloud-config.yaml.tftpl"), "/opt/geoguessme/config/compose.production.yaml")
    )
    error_message = "Cloud-init must disable password SSH and schedule backups."
  }
}
