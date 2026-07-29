#!/usr/bin/env bash

set -exuo pipefail

ARTIFACT_DIR=${ARTIFACT_DIR:=/tmp/artifacts}
INSTALLER_DIR=${INSTALLER_DIR:=${ARTIFACT_DIR}/installer}
PLUGIN_NAME="console-functions-plugin"
PLUGIN_NAMESPACE="console-functions-plugin"

function copyArtifacts {
  if [ -d "$ARTIFACT_DIR" ]; then
    echo "Copying artifacts from $(pwd)..."
    cp -r .e2e/results "${ARTIFACT_DIR}/" 2>/dev/null || true
    cp -r .e2e/report "${ARTIFACT_DIR}/" 2>/dev/null || true
  fi
}

trap copyArtifacts EXIT

# --- Credentials ---
set +x
BRIDGE_KUBEADMIN_PASSWORD="$(cat "${KUBEADMIN_PASSWORD_FILE:-${INSTALLER_DIR}/auth/kubeadmin-password}")"
export BRIDGE_KUBEADMIN_PASSWORD
set -x

BRIDGE_BASE_ADDRESS="$(oc get consoles.config.openshift.io cluster -o jsonpath='{.status.consoleURL}')"
export BRIDGE_BASE_ADDRESS

echo "Console URL: ${BRIDGE_BASE_ADDRESS}"

if [[ -z "${PLUGIN_PULL_SPEC:-}" ]]; then
  echo "Error: PLUGIN_PULL_SPEC is not set. It should be injected by ci-operator as a dependency."
  exit 1
fi

# --- Deploy plugin ---
oc new-project "${PLUGIN_NAMESPACE}" || oc project "${PLUGIN_NAMESPACE}"

oc apply -n "${PLUGIN_NAMESPACE}" -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ${PLUGIN_NAME}
  annotations:
    service.alpha.openshift.io/serving-cert-secret-name: ${PLUGIN_NAME}-cert
spec:
  selector:
    app: ${PLUGIN_NAME}
  ports:
  - port: 9443
    targetPort: 9443
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${PLUGIN_NAME}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${PLUGIN_NAME}
  template:
    metadata:
      labels:
        app: ${PLUGIN_NAME}
    spec:
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: plugin
        image: ${PLUGIN_PULL_SPEC}
        args: ["--https-port=9443"]
        ports:
        - containerPort: 9443
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: [ALL]
        volumeMounts:
        - name: cert
          mountPath: /var/cert
          readOnly: true
      volumes:
      - name: cert
        secret:
          secretName: ${PLUGIN_NAME}-cert
---
apiVersion: console.openshift.io/v1
kind: ConsolePlugin
metadata:
  name: ${PLUGIN_NAME}
spec:
  displayName: ${PLUGIN_NAME}
  backend:
    type: Service
    service:
      name: ${PLUGIN_NAME}
      namespace: ${PLUGIN_NAMESPACE}
      port: 9443
      basePath: /
  proxy:
  - alias: backend
    endpoint:
      type: Service
      service:
        name: ${PLUGIN_NAME}
        namespace: ${PLUGIN_NAMESPACE}
        port: 9443
EOF

echo "Waiting for plugin deployment rollout..."
oc rollout status deployment/"${PLUGIN_NAME}" -n "${PLUGIN_NAMESPACE}" --timeout=300s

echo "Enabling plugin on the console..."
oc patch consoles.operator.openshift.io cluster \
  --type=json \
  --patch='[{"op":"add","path":"/spec/plugins/-","value":"'"${PLUGIN_NAME}"'"}]'

echo "Restarting console pods to pick up the plugin..."
oc rollout restart deployment/console -n openshift-console
oc rollout status deployment/console -n openshift-console --timeout=300s

# --- Install deps and run tests ---
echo "Installing dependencies..."
yarn install --immutable

echo "Installing Playwright browsers..."
npx playwright install --with-deps chromium

echo "Running Playwright e2e tests..."
yarn test:e2e
