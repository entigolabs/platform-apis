---
sidebar_position: 2
---

# Permission Boundaries
*Status*: draft

Entigo platform makes use of cloud providers privilege delegation functionality, to manage cloud resources. Users may grant Entigo access to their cloud resources to manage these resources through Entigo Platform. Most cloud providers use role based access control (RBAC) by default. This means that you have roles with privileges across cloud services of given type. For example Database Administrator role to provision and manage database instances. Now, in order to delegate database management to Entigo Platvorm, user needs to grant Entigo the Database Administrator role. But through that Entigo could gain access to databases that are deployed to the same cloud account but are not managed through Entigo Platform. This situation violates the minimal privileges principle and may not be acceptable to security cautious organizations. 

AWS Permission Boundaries make it possible to limit a role maximum privileges. Entigo Platform makes use of AWS Permission Boundaries to limit access to resources with a tag `entigo.com/management-policy` set to `observed` of `full`

Azure and GCP have slightly different means for implementing similar limitation. Implementing Permission Boundaries on these platforms is still under consideration. 
