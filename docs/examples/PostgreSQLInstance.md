### Basic Instance
A minimal PostgreSQL instance definition showing the required fields only.

```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: basic-instance
spec:
  allocatedStorage: 20
  engineVersion: "18.1"
  instanceType: db.t4g.micro
```

### High Availability Instance
An example of a PostgreSQL instance configured with replication for high availability.

```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: ha-instance
spec:
  allocatedStorage: 20
  engineVersion: "18.1"
  instanceType: db.t4g.micro
  multiAZ: true
```
