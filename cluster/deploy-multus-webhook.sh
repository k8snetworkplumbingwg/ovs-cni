#!/bin/bash
#
# Copyright 2018 Red Hat, Inc.
# Copyright 2018 Intel Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -e

tmp=$(mktemp -d)

# generate private key
echo "Generating private RSA key..."
openssl genrsa -out ${tmp}/webhook-key.pem 2048 >/dev/null 2>&1

# generate CSR
echo "Generating CSR configuration file..."
cat <<EOF >> ${tmp}/webhook.conf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = multus-webhook-service
DNS.2 = multus-webhook-service.default
DNS.3 = multus-webhook-service.default.svc
EOF
openssl req -new -key ${tmp}/webhook-key.pem -subj "/CN=multus-webhook-service.default.svc" -out ${tmp}/server.csr -config ${tmp}/webhook.conf

# push CSR to Kubernetes API server
echo "Sending CSR to Kubernetes..."
csr_name="multus-webhook-service.default"
./cluster/kubectl.sh delete --ignore-not-found csr ${csr_name} >/dev/null 2>&1
cat <<EOF | ./cluster/kubectl.sh create -f -
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: ${csr_name}
spec:
  request: $(cat ${tmp}/server.csr | base64 -w0)
  groups:
  - system:authenticated
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

# approve certificate
echo "Approving CSR..."
./cluster/kubectl.sh certificate approve ${csr_name}

# wait for the cert to be issued
echo -n "Waiting for the certificate to be issued..."
cert=""
for sec in $(seq 15); do
  cert=$(./cluster/kubectl.sh get csr ${csr_name} -o jsonpath='{.status.certificate}')
  if [[ $cert != "" ]]; then
    echo -e "\nCertificate issued succesfully."
    echo $cert | base64 --decode > ${tmp}/webhook-cert.pem
    break
  fi
  echo -n "."; sleep 1
done
if [[ $cert == "" ]]; then
  echo -e "\nError: certificate not issued. Verify that the API for signing certificates is enabled."
  exit
fi

# create secret
echo "Creating secret..."
./cluster/kubectl.sh delete --ignore-not-found secret "multus-webhook-secret"
./cluster/kubectl.sh create secret generic --from-file=key.pem=${tmp}/webhook-key.pem --from-file=cert.pem=${tmp}/webhook-cert.pem "multus-webhook-secret"

cat <<EOF | ./cluster/kubectl.sh create -f -
---
apiVersion: admissionregistration.k8s.io/v1beta1
kind: ValidatingWebhookConfiguration
metadata:
  labels:
    app: multus-webhook
  name: multus-webhook-config
webhooks:
- clientConfig:
    caBundle: ${cert}
    service:
      name: multus-webhook-service
      namespace: default
      path: /validate
  failurePolicy: Fail
  name: multus-webhook.k8s.cni.cncf.io
  rules:
  - apiGroups:
    - k8s.cni.cncf.io
    apiVersions:
    - v1
    resources:
    - network-attachment-definitions
    operations:
    - CREATE
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: multus-webhook
  name: multus-webhook-deployment
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multus-webhook
  template:
    metadata:
      labels:
        app: multus-webhook
    spec:
      containers:
      - name: multus-webhook
        image: phoracek/multus-webhook
        command:
        - /webhook/webhook
        args:
        - --bind-address=0.0.0.0
        - --port=443
        - --tls-private-key-file=/webhook/tls/key.pem
        - --tls-cert-file=/webhook/tls/cert.pem
        volumeMounts:
        - mountPath: /webhook/tls
          name: multus-webhook-secret
          readOnly: True
        imagePullPolicy: IfNotPresent
      volumes:
      - name: multus-webhook-secret
        secret:
          secretName: multus-webhook-secret
---
apiVersion: v1
kind: Service
metadata:
  name: multus-webhook-service
  labels:
    app: multus-webhook
  namespace: default
spec:
  ports:
  - port: 443
    targetPort: 443
  selector:
    app: multus-webhook
EOF
