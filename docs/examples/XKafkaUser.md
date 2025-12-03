### Basic Kafka user

```yaml
---
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-a
  namespace: default
spec:
  clusterName: test-crossplane-cluster
```

### Kafka user with Consumer Group ACL's

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-b
  namespace: team-b
spec:
  clusterName: test-crossplane-cluster
  consumerGroups:
    - alpha
    - gamma
```

### Kafka user with Topic ACL's

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-b
  namespace: team-b
spec:
  clusterName: test-crossplane-cluster
  acls:
    - topic: topic-a
      operation: Read
    - topic: topic-b
      operation: Write
```
