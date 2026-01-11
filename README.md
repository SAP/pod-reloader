# Reload Kubernetes Pods upon Configuration Changes

[![REUSE status](https://api.reuse.software/badge/github.com/SAP/pod-reloader)](https://api.reuse.software/info/github.com/SAP/pod-reloader)

## About this project

It is a common problem that Kubernetes workloads (pods) referencing configuration in form of config maps or secrets (as environment variables or volumes) are not automatically notified when this configuration changes. In the case of (non-subpath) volume references, Kubernetes indeed updates the corresponding mounts inside the pod's containers, but in order to have an effect, the running workload would still need to actively reread the contents. Which is not fulfilled for many or most applications. In the case of environment variable references or subpath volume mounts, config map or secret changes are not propagated to the pods at all.

This is where the operator provided by this repository comes into play. It allows to declare configuration dependencies via the following annotations on deployments, stateful sets and daemon sets:

- `pod-reloader.cs.sap.com/configmaps`
- `pod-reloader.cs.sap.com/secrets`

containing a comma-separated list of the names of config maps resp. secrets in the same namespace as the annotated object.

The operator will maintain the annotation `pod-reloader.cs.sap.com/config-hash` on the pod template (i.e. `.spec.template`) of the according deployment, stateful set, daemon set.
The value of this annotation is ensured to be an up-to-date hash calculated from all the referenced config maps and secrets. That way, whenever the referenced configuration changes, the related pods will be restarted according to the restart/upgrade policy maintained on the owning deployment, stateful set or daemon set.

**Note:** there are other projects (e.g. [https://github.com/stakater/Reloader](https://github.com/stakater/Reloader)) providing a similar functionality, but we found that they are not properly handling updates of the owning deployment (or stateful set, daemon set), because those updates would typically remove the config hash annotation previously inserted by the operator. Which may lead to flickering pod restart behavior. Other than the evaluated community projects, the operator provided by this repository uses a mutating webhook to consistently maintain the config hash annotation, and is therefore not prone to the described race condition.

An example deployment may look as follows:

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deployment
  annotations:
    pod-reloader.cs.sap.com/secrets: my-secret,my-other-secret
    pod-reloader.cs.sap.com/configmaps: my-configmap
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: main
        image: ubuntu
        command:
        - sleep
        - infinity
        env:
        - name: KEY_FROM_SECRET
          valueFrom:
            secretKeyRef:
              name: my-secret
              key: somekey
        - name: KEY_FROM_CONFIGMAP
          valueFrom:
            configMapKeyRef:
              name: my-configmap
              key: somekey
        volumeMounts:
        - name: data
          mountPath: /data
          readOnly: true
      volumes:
      - name: data
        secret:
          secretName: my-other-secret
```

Whenever one of the secrets `my-secret`, `my-other-secret`, or the config map `my-configmap` changes, the pods of the deployment will be recreated.

## How it works in detail

To every deployment, stateful set or daemon set that
- is selected by the pod-reloader's `MutatingWebhookConfiguration` and
- has at least one of the annotations `pod-reloader.cs.sap.com/configmaps`, `pod-reloader.cs.sap.com/secrets`

the pod-reloader webhook adds an annotation `pod-reloader.cs.sap.com/config-hash` to the pod template of the corresponding workload set, containing a digest value,
calculated from the content of all referenced config maps and secrets. If changing, this triggers a rollout of the workload set.

The `MutatingWebhookConfiguration` registering the pod-reloader webhook should
- match deployments, stateful sets and daemon sets and
- subscribe to `CREATE` and `UPDATE` events and
- may select (include/exclude) certain namespaces and objects through their labels.

When updating the deployment, stateful set, or daemon set, clients may set the annotation `pod-reloader.cs.sap.com/config-hash`
(yes, that is is the same annotation which is later added by the webhook on the pod template spec, but on the workload set).
If the annotation is present, its value must equal the digest calculated by the webhook, and it will be purged by the webhook (so it can be
considered as a dummy annotation, just triggering the webhook). Otherwise, the webhook rejects the request.
This way the client can ensure that the reload happens for the exact same config change it has observed. Of course, in case of an error, the client
should perform some retries.

Now, pod-reloader itself runs a reconcile/watch loop on config maps and secrets. Whenever a config map or secret (referenced through a workload set through the annotations `pod-reloader.cs.sap.com/configmaps` or `pod-reloader.cs.sap.com/secrets`) is created, updated or deleted, then this watch handler
updates the workload set with the 'dummy' annotation `pod-reloader.cs.sap.com/config-hash` described above, triggering an immediate execution of the webhook.

## Requirements and Setup

The recommended deployment method is to use the [Helm chart](https://github.com/sap/pod-reloader-helm):

```bash
helm upgrade -i pod-reloader oci://ghcr.io/sap/pod-reloader-helm/pod-reloader
```

## Documentation
 
The API reference is here: [https://pkg.go.dev/github.com/sap/pod-reloader](https://pkg.go.dev/github.com/sap/pod-reloader).

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/pod-reloader/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2025 SAP SE or an SAP affiliate company and pod-reloader contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/pod-reloader).
