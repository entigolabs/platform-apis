### Basic User
A minimal PostgreSQL User definition showing the required fields only.

```yaml
apiVersion: database.example.org/v1alpha1
kind: PostgreSQLUser
metadata:
  name: basic-user
spec:
  instanceRef:
    name: basic-instance
```