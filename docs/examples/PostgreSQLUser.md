### Basic User
A minimal PostgreSQL User definition showing the required fields only.

```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLUser
metadata:
  name: basic-user
spec:
  instanceRef:
    name: basic-instance
```

### Prerequisites
The PostgreSQL User requires the following resource applied:

#### PostgreSQLInstance
```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: basic-instance
spec:
  storageGB: 20
  version: "17.2"
```