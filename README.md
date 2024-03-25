# b3scale-operator

## Installation

1. Deploy CRD
    + `kubectl apply -f kubernetes/crd.yaml`
2. Configure Operator by modifying the Secret in `kubernetses/operator.yaml`
3. Deploy Operator
    + `kubectl apply -f kubernetes/operator.yaml`

## Usage

```yaml
apiVersion: b3scale.io/v1
kind: BBBFrontend
metadata:
  name: frontend-full-settings
spec:
  settings:
    defaultPresentation:
      url: ""
      force: true
    requiredTags:
      - "bbb_24_mediasoup"
    createOverrideParams:
       allowStartStopRecording: "false"
    createDefaultParams:
       allowStartStopRecording: "false"
       disabledFeatures: "chat,captions"
```

This will create a frontend instance with the name `b3scale-operator-testeroni`. Add a secret with credentials to the `BBBFrontend` resource. An full example is included in the `./kubernetes/test-bbb.yaml` file.

```yaml
apiVersion: b3scale.io/v1
kind: BBBFrontend
metadata:
  name: frontend-provided-credentials
spec:
  credentials:
    frontend: bbb.example.com
    secretRef:
      name: frontend-provided-credentials-secret
      key: "FRONTEND_API_SECRET"
  settings: {}
```
