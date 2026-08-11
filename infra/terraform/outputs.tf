output "server_ipv4" {
  description = "Recovery-console reference only; inbound traffic is denied."
  value       = hcloud_server.app.ipv4_address
}

output "tunnel_id" {
  value = cloudflare_zero_trust_tunnel_cloudflared.app.id
}
