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
  description = "Only human identity allowed into Cloudflare Access applications."
  type        = string
  default     = "jeancollette138@gmail.com"

  validation {
    condition     = var.access_email == "jeancollette138@gmail.com"
    error_message = "The initial Access policy must remain scoped to the approved address."
  }
}

variable "dev_health_token_id" {
  description = "Cloudflare resource UUID (result.id, not client_id) for the development health service token. The token is created outside Terraform and its secret never enters state."
  type        = string

  validation {
    condition     = can(regex("^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$", var.dev_health_token_id))
    error_message = "dev_health_token_id must be the service token's UUID-shaped result.id, not its client_id."
  }
}

variable "dev_deploy_token_id" {
  description = "Cloudflare resource UUID (result.id, not client_id) for the development deployment service token. The token is created outside Terraform and its secret never enters state."
  type        = string

  validation {
    condition     = can(regex("^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$", var.dev_deploy_token_id))
    error_message = "dev_deploy_token_id must be the service token's UUID-shaped result.id, not its client_id."
  }
}

variable "prod_deploy_token_id" {
  description = "Cloudflare resource UUID (result.id, not client_id) for the production deployment service token. The token is created outside Terraform and its secret never enters state."
  type        = string

  validation {
    condition     = can(regex("^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$", var.prod_deploy_token_id))
    error_message = "prod_deploy_token_id must be the service token's UUID-shaped result.id, not its client_id."
  }
}

variable "operator_email" {
  description = "Operator mailbox that receives DMARC aggregate/failure reports via Cloudflare Email Routing."
  type        = string
  default     = "jeancollette138@gmail.com"
}

variable "enable_dmarc_forwarding" {
  description = "Create the DMARC forwarding rule only after the operator destination address has been verified in Cloudflare."
  type        = bool
  default     = false
}

variable "spf_record" {
  description = "Single combined SPF TXT value authorizing Brevo and Cloudflare Email Routing forwarding."
  type        = string
  default     = "v=spf1 include:spf.brevo.com include:_spf.mx.cloudflare.net ~all"

  validation {
    condition = (
      startswith(var.spf_record, "v=spf1") &&
      strcontains(var.spf_record, "_spf.mx.cloudflare.net") &&
      length(regexall("v=spf1", var.spf_record)) == 1
    )
    error_message = "The SPF record must be a single v=spf1 value that includes _spf.mx.cloudflare.net for Cloudflare Email Routing."
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

  validation {
    condition = alltrue([
      for record in values(var.brevo_dns_records) :
      !(record.type == "TXT" && startswith(record.content, "v=spf1"))
    ])
    error_message = "Do not publish a second SPF record through brevo_dns_records; set the single combined SPF value in spf_record instead."
  }
}
