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

A PostgreSQL User definition  with role grant.
```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLUser
metadata:
  name: user-example
spec:
  instanceRef:
    name: postgresql-example
  grant:
    roles:
    - example-role
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