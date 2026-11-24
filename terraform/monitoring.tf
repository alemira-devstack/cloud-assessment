resource "terraform_data" "monitoring" {
  depends_on = [terraform_data.k3d_cluster, terraform_data.argocd, terraform_data.keda]

  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    command     = <<-EOT
      set -euo pipefail
      helm repo add prometheus-community https://prometheus-community.github.io/helm-charts --force-update
      helm repo update prometheus-community
      helm upgrade --install monitoring prometheus-community/kube-prometheus-stack --version 93.9.0 --namespace monitoring --create-namespace --kubeconfig ../.kube/config --values monitoring-values.yaml --wait --timeout 15m
    EOT
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["bash", "-c"]
    command     = "helm uninstall monitoring --namespace monitoring --kubeconfig ../.kube/config || true"
  }
}
