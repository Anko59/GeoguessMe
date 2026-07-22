variable "cloudflare_account_id" {
  description = "Cloudflare account identifier."
  type        = string
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone identifier for geoguessme.com."
  type        = string
}

variable "admin_ssh_public_key" {
  description = "Operator SSH public key, reachable only through Access."
  type        = string
  sensitive   = true
}

variable "dev_ci_ssh_public_key" {
  description = "CI key restricted to the dev forced command."
  type        = string
  sensitive   = true
}

variable "production_ci_ssh_public_key" {
  description = "CI key restricted to the production forced command."
  type        = string
  sensitive   = true
}

variable "access_email" {
  description = "Only human identity allowed into the dev application."
  type        = string
  default     = "jeancollette138@gmail.com"

  validation {
    condition     = var.access_email == "jeancollette138@gmail.com"
    error_message = "The initial dev Access policy must remain scoped to the approved address."
  }
}

variable "domain" {
  type    = string
  default = "geoguessme.com"
}

variable "brevo_dns_records" {
  description = "SPF and DKIM records supplied by Brevo after domain verification."
  type = map(object({
    name    = string
    type    = string
    content = string
  }))
  default = {}

  validation {
    condition     = alltrue([for record in values(var.brevo_dns_records) : contains(["TXT", "CNAME"], record.type)])
    error_message = "Brevo records may only be TXT or CNAME records."
  }
}
