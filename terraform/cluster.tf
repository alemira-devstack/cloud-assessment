resource "terraform_data" "k3d_cluster" {
  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    command     = <<-EOT
      set -euo pipefail
      k3d cluster create assessment --servers 1 --agents 0 --no-lb --k3s-arg "--disable=traefik@server:0" --kubeconfig-update-default=false --kubeconfig-switch-context=false
      mkdir -p ../.kube
      k3d kubeconfig get assessment > ../.kube/config
    EOT
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["bash", "-c"]
    command     = "mkdir -p ../.kube && touch ../.kube/delete-decoy && KUBECONFIG=../.kube/delete-decoy k3d cluster delete assessment && rm -f ../.kube/config ../.kube/delete-decoy"
  }
}
