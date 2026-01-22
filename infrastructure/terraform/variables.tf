variable "hcloud_token" {
  description = "Hetzner Cloud API token"
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "SSH public key for VM access (leave empty to generate)"
  type        = string
  default     = ""
}

variable "k3s_token" {
  description = "Token for k3s cluster join (leave empty to generate)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "cluster_name" {
  description = "Name prefix for cluster resources"
  type        = string
  default     = "vultisig"
}

variable "master_server_type" {
  description = "Server type for master node"
  type        = string
  default     = "ccx13" # 2 dedicated vCPU, 8GB RAM (AMD64) - GHCR images are AMD64 only
}

variable "worker_server_type" {
  description = "Server type for worker nodes"
  type        = string
  default     = "ccx23" # 4 dedicated vCPU, 16GB RAM (AMD64) - GHCR images are AMD64 only
}

variable "regions" {
  description = "Hetzner regions to deploy workers"
  type        = list(string)
  default     = ["hel1"] # fsn1 out of cpx capacity, using nbg1
}
