### Basic Database
A minimal PostgreSQL Database definition showing the required fields only.

```yaml
apiVersion: database.example.org/v1alpha1
kind: PostgreSQLDatabase
metadata:
  name: basic-database
spec:
  owner: owner
  instanceRef:
    name: basic-instance
```