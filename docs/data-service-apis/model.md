# Model

Each data service is built from four parts:

| Part                   | Definition                                                                                       | Owned by | Example                                  |
| ---------------------- | ------------------------------------------------------------------------------------------------ | -------- | ---------------------------------------- |
| Core kind              | The resource a user requests, the same wherever it runs                                          | the user | a `PostgreSQLCluster`                    |
| Implementation         | The provider of a core kind on one platform: a cloud's managed service, or software in a cluster | Entigo   | RDS, Cloud SQL, CloudNativePG            |
| Provider configuration | The settings that exist only in one implementation, in a kind that implementation defines        | the user | a `PostgreSQLRDSConfig` with an RDS size |
| Class                  | A deployment's choice of implementation, its defaults and the values a user cannot change        | Entigo   | a `PostgreSQLClass` named `standard`     |

*The parts of a data service.*

A user writes a core kind that names a class, and may add a provider configuration for each
implementation it can run on. Each deployment applies the provider configuration of its class's
implementation, and the implementation turns the result into running resources.
