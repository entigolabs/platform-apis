### Basic Instance
A minimal PostgreSQL instance definition showing the required fields only.

```yaml
apiVersion: database.example.org/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: basic-instance
spec:
  storageGB: 10
  version: "14"
```
### High Availability Instance
An example of a PostgreSQL instance configured with replication for high availability.

```yaml
apiVersion: database.example.org/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: ha-instance
spec:
  storageGB: 100
  version: "15"
  replicas: 3
  backup:
    enabled: true
    schedule: "0 2 * * *"
```