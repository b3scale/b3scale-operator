# b3scale-operator

## Installation

1. Deploy CRD
    + `kubectl apply -f kubernetes/crd.yaml`
2. Configure Operator by modifing the Secret in `kubernetses/operator.yaml`
3. Deploy Operator
    + `kubectl apply -f kubernetes/operator.yaml`

## Usage

```yaml
apiVersion: b3scale.infra.run/v1
kind: BBBFrontend
metadata:
  name: frontend-full-settings
spec:
  settings:
    default_presentation:
      url: ""
      force: true
    required_tags:
      - "bbb_24_mediasoup"
    create_override_params:
       allowStartStopRecording: "false"
    create_default_params:
       allowStartStopRecording: "false"
       disabledFeatures: "chat,captions"
```

This will create a frontend instance with the name `b3scale-operator-testeroni`. This will also create a ConfigMap and a Secret with the same name containing the needed configuration options to use this in other deployments.

| Source    | Key             | Description                                                                                               | 
|-----------|-----------------|-----------------------------------------------------------------------------------------------------------|
| ConfigMap | FRONTEND_ID     | This contains the unique ID of the Frontend and is used to identify the Resource on the b3scale instance. |
| ConfigMap | FRONTEND_HOST   | This contains the b3scale host, that hosts the BBB Frontend                                               |
| Secret    | FRONTEND_KEY    | This contains the frontend key.                                                                           |
| Secret    | FRONTEND_SECRET | This contains the frontend secret.                                                                        |

There is also the possibility to add a secret with credentials to the `BBBFrontend` resource. An full example is included in the `./kubernetes/test-bbb.yaml` file.

```yaml
apiVersion: b3scale.infra.run/v1
kind: BBBFrontend
metadata:
  name: frontend-provided-credentials
spec:
  credentials:
    key: frontend-provided-credentials-api-key
    secretRef:
      name: frontend-provided-credentials-secret
      key: "FRONTEND_API_SECRET"
  settings: {}
```