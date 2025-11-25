import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# PostgreSQLDatabase

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [database.entigo.com/v1alpha1](#database.entigo.com/v1alpha1)

# database.entigo.com/v1alpha1

Resource Types:

- [PostgreSQLDatabase](#postgresqldatabase)




## PostgreSQLDatabase
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
      <td>PostgreSQLDatabase</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#postgresqldatabasespec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLDatabase.spec
<sup><sup>[↩ Parent](#postgresqldatabase)</sup></sup>





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
        <td><b><a href="#postgresqldatabasespecinstanceref">instanceRef</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>owner</b></td>
        <td>string</td>
        <td>
          Owner role name<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>extensions</b></td>
        <td>[]enum</td>
        <td>
          List of PostgreSQL extensions to enable<br/>
          <br/>
            <i>Enum</i>: postgis<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLDatabase.spec.instanceRef
<sup><sup>[↩ Parent](#postgresqldatabasespec)</sup></sup>





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

</TabItem>

<TabItem value="examples" label="Examples">
### Basic Database {#example-basic-database}
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
</TabItem>

</Tabs>
