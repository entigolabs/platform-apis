These manifests are used for testing the render function locally.
In project root folder use the command:
```shell
crossplane render compositions/repository/examples/repository.yaml compositions/repository/apis/repository-composition.yaml functions/artifact/render/function.yaml
```
With extra resources:
```sh
crossplane render -e functions/artifact/render/required-resources.yaml -o functions/artifact/render/observed-resources.yaml compositions/repository/examples/repository.yaml compositions/repository/apis/repository-composition.yaml functions/artifact/render/function.yaml
```
With observed resources:
```sh
crossplane render -e functions/artifact/render/required-resources.yaml -o functions/artifact/render/observed-resources.yaml compositions/repository/examples/repository.yaml compositions/repository/apis/repository-composition.yaml functions/artifact/render/function.yaml
```
```