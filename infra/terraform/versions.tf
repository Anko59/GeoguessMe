terraform {
  required_version = "= 1.15.8"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "= 5.22.0"
    }
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "= 1.66.1"
    }
    random = {
      source  = "hashicorp/random"
      version = "= 3.7.2"
    }
  }
}

provider "cloudflare" {}
provider "hcloud" {}
