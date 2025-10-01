# Developer Portal Function

Entigo developer portal Crossplane composition function for reconciling portal resources.

Based on a template for writing a [composition function][functions] in [Go][go].

To learn how to use this template:

* [Follow the guide to writing a composition function in Go][function guide]
* [Learn about how composition functions work][functions]
* [Read the function-sdk-go package documentation][package docs]

If you just want to jump in and get started:

1. Update `input/v1beta1/` to reflect your desired input (and run `go generate`)
1. Add your logic to `RunFunction` in `fn.go`
1. Add tests for your logic in `fn_test.go`

This template uses [Go][go], [Docker][docker], and the [Crossplane CLI][cli] to
build functions.

Generating updated object definitions:

```shell
# Run code generation - see input/generate.go, outputs to package/input directory
go generate ./...
```
Using those objects as CompositeResourceDefinitions requires replacing the `apiVersion` and `kind` in the generated files with the correct values, e.g.:
```yaml
apiVersion: apiextensions.crossplane.io/v2
kind: CompositeResourceDefinition
```
Also `storage: true` needs to be replaced with `referenceable: true`

Required environment variables:
* AWS_PROVIDER - name of the Crossplane aws provider
* IMAGE_PULL_SECRETS - name of the image pull secrets for generated containers
* ISTIO_GATEWAY - name of the Istio gateway for virtual services

```shell
# Run local function for development
go run . --insecure --debug

# Run tests - see fn_test.go
go test ./...

# Test rendering of function package inside example directory, use the manifests from example directory
# Use flags -e and -o to add extra resources and observed resources for testing
crossplane render -r webapp-claim.yaml webapp-composition.yaml functions.yaml
```

Running the function in a local cluster. Steps after applying the function to the cluster:
```shell
# Build the function's runtime image - see Dockerfile
docker build . --tag=entigolabs/developer-portal-function:latest

# You can try to force the pod to use the built image by tagging it with the same tag as the function image
docker tag entigolabs/developer-portal-function:latest entigolabs/developer-portal-function:<matching-remote-tag>
# After that scale the function deployment to 0 and back to 1 to force a restart, e.g
kubectl scale deployment -n crossplane-system developer-portal-function-<randomId> --replicas 0

# Alternatively, patch the deployment image, but crossplane will reconcile the deployment but a pod with the updated image should start
# This only works if the function started with a not working image, otherwise the deployment will be updated with the latest image from the function package
kubectl patch deployment -n crossplane-system \
  --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value": "index.docker.io/entigolabs/developer-portal-function:latest"}]' \
  developer-portal-function-<random-id>
```

## Cluster requirements

* [Crossplane v1.4.0 or later](https://docs.crossplane.io/latest/software/install/)
* [Crossplane aws provider](https://github.com/crossplane-contrib/provider-aws)

## Cluster installation

Example kubernetes yaml files are in the example subdirectory.

This function requires a custom [DeploymentRuntimeConfig](example/runtime.yaml), with required environment variables:
* ISTIO_GATEWAY - Istio gateway name for virtual services
* KUBERNETES_PROVIDER - Crossplane kubernetes provider name
* AWS_PROVIDER - Crossplane aws provider name
* IMAGE_PULL_SECRETS - Comma separated list of image pull secrets for generated containers

### Installation steps

1. Install Crossplane aws provider `kubectl apply -f example/aws-provider/aws-provider.yaml`
2. Create a temporary credentials file aws-credentials.txt for the aws-provider (not suitable for permanent solution) `vi aws-credentials.txt`
    ```
    [default]
    aws_access_key_id = <aws_access_key_id>
    aws_secret_access_key = <aws_secret_access_key>
    aws_session_token = <aws_session_token>
    ```
3. Create a secret with aws credentials for the aws-provider `kubectl create secret generic crossplane-aws -n crossplane-system --from-file=creds=./aws-credentials.txt`
4. Install the aws-provider-config `kubectl apply -f example/aws-provider/aws-provider-config.yaml`
5. Install the runtime `kubectl apply -f example/runtime.yaml`
6. Create a regcred for function package if needed `kubectl create secret docker-registry regcred --docker-server=https://index.docker.io/ --docker-username=<your-name> --docker-password=<your-pword> --docker-email=<your-email> -n crossplane-system`
7. Install the function `kubectl apply -f example/functions.yaml`
   * Make sure the package version is correct, by changing the image tag in the functions.yaml. You can find the latest tag from [Docker Hub](https://hub.docker.com/r/entigolabs/developer-portal-function/tags)
8. Install the xrd's from example directory
   ```
       kubectl apply -f example/web-access/xrd.yaml \
           -f example/webapp/xrd.yaml \
           -f example/cronjob/xrd.yaml \
           -f example/repository/xrd.yaml \
           -f example/postgreSQL/xrd.yaml
   ```
9. Make sure the xrd is ready `kubectl get xrd`
10. Install the compositions from example directory
    ```
        kubectl apply -f example/web-access/composition.yaml \
            -f example/webapp/composition.yaml \
            -f example/cronjob/composition.yaml \
            -f example/repository/composition.yaml \
            -f example/postgreSQL/composition.yaml
    ```
11. Make sure functions and providers are ready:
    * ```kubectl get providers.pkg.crossplane.io```
    * ```kubectl get functions.pkg.crossplane.io```

[functions]: https://docs.crossplane.io/latest/concepts/composition-functions
[go]: https://go.dev
[function guide]: https://docs.crossplane.io/knowledge-base/guides/write-a-composition-function-in-go
[package docs]: https://pkg.go.dev/github.com/crossplane/function-sdk-go
[docker]: https://www.docker.com
[cli]: https://docs.crossplane.io/latest/cli

### Uninstall steps

1. Delete crossplane-system namespace: ```helm uninstall crossplane -n crossplane-system```
2. Ensure namespace deletion: ```kubectl delete ns crossplane-system```
    * Ensure that crossplane-system namespace gets deleted. If not then delete its namespace objects manually with ```--force``` flag.
3. Delete provider and providerconfigs: 
    * ```kubectl get providers.pkg.crossplane.io``` and delete listed providers.
    * ```kubectl get providerconfigs```and delete listed providerconfigs.
    * If deleting hangs then remove finalizer: ```kubectl patch <providers.pkg.crossplane.io or providerconfig> <name> -p '{"metadata":{"finalizers":null}}' --type=merge```
4. Delete functions: ```kubectl delete functions.pkg.crossplane.io developer-portal-function```. There might be more functions to delete.
