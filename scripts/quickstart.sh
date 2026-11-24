#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

docker build -f Dockerfile.producer -t assessment-producer:1.0.0 --provenance=false --sbom=false .
docker build -f Dockerfile.worker -t assessment-worker:1.0.0 --provenance=false --sbom=false .

terraform -chdir=terraform init
terraform -chdir=terraform apply -auto-approve

k3d image import assessment-producer:1.0.0 --cluster assessment --mode direct
k3d image import assessment-worker:1.0.0 --cluster assessment --mode direct

kubectl --kubeconfig .kube/config apply -f argocd/application-test.yaml
kubectl --kubeconfig .kube/config apply -f argocd/application-prod.yaml
