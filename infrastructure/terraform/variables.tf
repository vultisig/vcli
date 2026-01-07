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
  default     = "cax11" # 2 vCPU, 4GB RAM (ARM)
}

variable "worker_server_type" {
  description = "Server type for worker nodes"
  type        = string
  default     = "cax21" # 4 vCPU, 8GB RAM (ARM)
}

variable "regions" {
  description = "Hetzner regions to deploy workers"
  type        = list(string)
  default     = ["fsn1"] # Reduced for account with low IP quota
}
