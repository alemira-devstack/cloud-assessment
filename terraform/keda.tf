resource "terraform_data" "keda" {
  depends_on = [terraform_data.k3d_cluster, terraform_data.argocd]

  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    command     = <<-EOT
      set -euo pipefail
      helm repo add kedacore https://kedacore.github.io/charts --force-update
      helm repo update kedacore
      helm upgrade --install keda kedacore/keda --version 2.21.1 --namespace keda --create-namespace --kubeconfig ../.kube/config --wait --timeout 10m
    EOT
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["bash", "-c"]
    command     = "helm uninstall keda --namespace keda --kubeconfig ../.kube/config || true"
  }
}
