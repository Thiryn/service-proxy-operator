# service-proxy-operator
This `service-proxy-operator` is a simple Operator to allow deploying a Nginx proxy to a Kubernetes ingress to services endpoints.

## Description
This is a sample project made in a couple of days for interview purposes.
This project is not meant to be a full Kubernetes Operator and only outlines the basic features of an Operator. It is
based off the [Operator Framework SDK](https://operatorframework.io). Operator Framework defines [capability levels](https://operatorframework.io/operator-capabilities/).
Per this framework, the `service-proxy-operator` is only a Level 1 operator, with some Level 2 features.

## Features
The `service-proxy-operator` uses a `ServiceProxy` Custom Resource Definition to deploy a Nginx proxy Deployment and attached service for the services listed in the CRD.
This allows to expose all the services from a single endpoint.

Every service defines endpoints such as: `[service].[namespace].svc.cluster.local`. Given two services `foo` and `bar` in the `default `namespace`,
you can create the ServiceProxy CRD such as:
```yaml
apiVersion: cache.service-proxy-operator.local/v1alpha1
kind: ServiceProxy
metadata:
  name: serviceproxy-sample
spec:
  serviceNames:
    - bar
    - foo
```

The Operator will then create the necessary resources, to allow to query each service at `http://serviceproxy-sample-service.default.svc.cluster.local/[foo|bar]/`.
It also creates an Ingress Object pointing to that service allowing to reach the services from outside the cluster.

If you have an ingress gateway setup to your cluster, you can then query your sevice like `curl http://[cluster]/foo/`

## Operator Capabilities
This demonstration operator only does simple tasks such as:
- Create the necessary resource (Deployment, Service, ConfigMap, Ingress) upon creating the `ServiceProxy` CR.
- Update the ConfigMap whenever the CR changes (to apply)

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

You can use `./hack/kind-start.sh` to start a kind cluster with a `localhost:80` ingress gateway, along with a local image registry.
### Running on the cluster
The following `Makefile` makes use of the local image registry by default. Overwrite it by setting `IMG=<some-registry>/service-proxy-operator:tag` in `make` commands.
1. Build and push the operator:
```shell
make docker-build docker-push
```
1. Deploy to the cluster in the current kubernetes context:
```shell
make deploy
```
1. Install sample Instances of Custom Resources:
The sample CR deploys [`agnhost`](https://pkg.go.dev/k8s.io/kubernetes/test/images/agnhost) for testing purposes

```sh
kubectl apply -f config/samples/
```
1. Query the services

The Hostnames are the same as we're using two services pointing to the same pod.
```shell
$> curl localhost/agnhost/hostname
agnhost-55755b8ff-kxhg8
$> curl localhost/other-agnhost/hostname
agnhost-55755b8ff-kxhg8
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

