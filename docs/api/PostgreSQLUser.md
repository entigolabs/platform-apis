import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# PostgreSQLUser

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [database.entigo.com/v1alpha1](#database.entigo.com/v1alpha1)

# database.entigo.com/v1alpha1

Resource Types:

- [PostgreSQLUser](#postgresqluser)




## PostgreSQLUser
<sup><sup>[↩ Parent](#database.entigo.com/v1alpha1 )</sup></sup>








<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>database.entigo.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>PostgreSQLUser</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#postgresqluserspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLUser.spec
<sup><sup>[↩ Parent](#postgresqluser)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#postgresqluserspecinstanceref">instanceRef</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>createDb</b></td>
        <td>boolean</td>
        <td>
          Allow user to create new databases. Default false.<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>createRole</b></td>
        <td>boolean</td>
        <td>
          Allow user to create new users. Default false.<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#postgresqluserspecgrant">grant</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>inherit</b></td>
        <td>boolean</td>
        <td>
          Inherit privileges from granted roles. Default true.<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>login</b></td>
        <td>boolean</td>
        <td>
          Enable user login. Default true.<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLUser.spec.instanceRef
<sup><sup>[↩ Parent](#postgresqluserspec)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          Name of the database instance the db should be created in<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### PostgreSQLUser.spec.grant
<sup><sup>[↩ Parent](#postgresqluserspec)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>roles</b></td>
        <td>[]string</td>
        <td>
          An array of role names to be granted.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>

<TabItem value="examples" label="Examples">
### Basic User {#example-basic-user}
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

### Prerequisites {#example-prerequisites}
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
</TabItem>

</Tabs>
