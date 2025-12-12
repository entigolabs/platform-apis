### Observed MSK Cluster

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: MSK
metadata:
  name: "{msk-cluster-name}"
spec:
  clusterARN: "arn:aws:kafka:{region}:{account}:cluster/{msk-cluster-name}/{uuid}"
```
