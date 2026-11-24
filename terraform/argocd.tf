resource "terraform_data" "argocd" {
  depends_on = [terraform_data.k3d_cluster]

  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    command     = <<-EOT
      set -euo pipefail
      helm repo add argo https://argoproj.github.io/argo-helm --force-update
      helm repo update argo
      sed -i 's/host.docker.internal/127.0.0.1/' ../.kube/config
      helm upgrade --install argocd argo/argo-cd --version 10.19.0 --namespace argocd --create-namespace --kubeconfig ../.kube/config --wait --timeout 10m
    EOT
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["bash", "-c"]
    command     = "helm uninstall argocd --namespace argocd --kubeconfig ../.kube/config || true"
  }
}
