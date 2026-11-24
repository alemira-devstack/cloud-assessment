#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

kubectl --kubeconfig .kube/config apply -f monitoring/grafana-dashboard.yaml
kubectl --kubeconfig .kube/config apply -f monitoring/prometheus-rule.yaml
helm template assessment charts/assessment --namespace test --show-only templates/podmonitor.yaml | kubectl --kubeconfig .kube/config apply -f -

mkdir -p .kube

start_forward() {
  local name="$1"
  local namespace="$2"
  local service="$3"
  local local_port="$4"
  local remote_port="$5"

  if (: >/dev/tcp/127.0.0.1/"${local_port}") >/dev/null 2>&1; then
    echo "port-forward already running on ${local_port}"
    return 0
  fi

  set -m || true
  nohup kubectl --kubeconfig .kube/config port-forward -n "$namespace" "svc/${service}" "${local_port}:${remote_port}" >".kube/port-forward-${name}.log" 2>&1 &
  disown || true
}

start_forward grafana monitoring monitoring-grafana 3000 80
start_forward argocd argocd argocd-server 8443 443
start_forward prometheus monitoring monitoring-kube-prometheus-prometheus 9090 9090

# Grafana 13 stores the login password in its own database. The Kubernetes secret does not update that database.
MSYS_NO_PATHCONV=1 kubectl --kubeconfig .kube/config -n monitoring exec deploy/monitoring-grafana -c grafana -- /usr/share/grafana/bin/grafana cli admin reset-admin-password admin >.kube/grafana-password.log 2>&1

echo "Grafana     http://grafana.localtest.me:3000/d/assessment/scale"
echo "Argo CD     https://argocd.localtest.me:8443"
echo "Prometheus  http://prometheus.localtest.me:9090/alerts"
echo "Grafana and Argo CD: admin / admin. Prometheus has no login."
echo "These names resolve to this machine. Ports: Grafana 3000, Argo CD 8443, Prometheus 9090. Port 8080 stays the producer test."
echo "The terminal 500-job commands stay separate; this script only opens the screens."

sleep 1
cmd.exe //c start "" "http://grafana.localtest.me:3000/d/assessment/scale" >/dev/null 2>&1 || true
cmd.exe //c start "" "https://argocd.localtest.me:8443" >/dev/null 2>&1 || true
cmd.exe //c start "" "http://prometheus.localtest.me:9090/alerts" >/dev/null 2>&1 || true
