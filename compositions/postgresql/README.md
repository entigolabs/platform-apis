# PostgreSQL APIs



## PostgreSQLInstance

Templeted objects:
1. SecurityGroup - ec2.aws.m.upbound.io
   1. SecurityGroupRule - ec2.aws.m.upbound.io
2. Instance - rds.aws.m.upbound.io
3. ExternalSecret - external-secrets.io

Required Resources (all are Infralib managed and Cluster scoped):
- ClusterSecretStore - external-secrets.io
- SubnetGroup - rds.aws.upbound.io
- Key - kms.aws.upbound.io

