### Basic Database
A minimal PostgreSQL Database definition showing the required fields only.

```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLDatabase
metadata:
  name: basic-database
spec:
  owner: owner
  instanceRef:
    name: basic-instance
```

### Prerequisites
The PostgreSQL Database requires the following resources applied:

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

#### PostgreSQLUser (Owner Role)
```yaml
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLUser
metadata:
  name: owner
spec:
  instanceRef:
    name: basic-instance
  createDb: true
  createRole: true
```