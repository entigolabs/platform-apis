# PostgreSQL APIs



## PostgreSQLInstance

### Templeted objects
1. SecurityGroup - ec2.aws.m.upbound.io
   1. SecurityGroupRule - ec2.aws.m.upbound.io
2. Instance - rds.aws.m.upbound.io
3. ExternalSecret - external-secrets.io

ExternalSecret will create a Secret object that contains:
- admin user name, defaults to dbadmin
- admin user password
- database address (fqdn)
- database port


### Required Resources 
All are Infralib managed and Cluster scoped:
- ClusterSecretStore - external-secrets.io
- SubnetGroup - rds.aws.upbound.io
- Key - kms.aws.upbound.io

### Naming convention
All templated resources, except Secret follow similar pattern as Kubernetes pods in a Depliyment/ReplicaSet: 
```
<instance metadata.name>-<sha>
```

Secret name should follow a naming convention that is predictable for the user:
```
<instance metadata.name>-<db admin user name>
```

The API should check if a conflicting secret with that name exists before creating the resource and fail with a clear error indicating a naming conflict. 

Resources in AWS should follow the same naming or user facing identifier convention as templated resources. 
