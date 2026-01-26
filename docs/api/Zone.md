import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# Zone

:::note Tiers
This feature is available for the following tiers: **Premium**.
:::


Packages:

- [tenancy.entigo.com/v1alpha1](#tenancy.entigo.com/v1alpha1)

# tenancy.entigo.com/v1alpha1

Resource Types:

- [Zone](#zone)




## Zone
<sup><sup>[↩ Parent](#tenancy.entigo.com/v1alpha1 )</sup></sup>








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
      <td>tenancy.entigo.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>Zone</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#zonespec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Zone.spec
<sup><sup>[↩ Parent](#zone)</sup></sup>





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
        <td><b><a href="#zonespecnamespacesindex">namespaces</a></b></td>
        <td>[]object</td>
        <td>
          List of namespaces to manage as part of the Zone. At least one namespace must be specified.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#zonespecpoolsindex">pools</a></b></td>
        <td>[]object</td>
        <td>
          List of node pool configurations for cluster-autoscaler<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#zonespecappproject">appProject</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>clusterPermissions</b></td>
        <td>boolean</td>
        <td>
          Enable cluster-level permissions for the zone. Value true will allow users in spec.appProject.contributorGroups to escalate privileges to Kubernetes administrator.<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Zone.spec.namespaces[index]
<sup><sup>[↩ Parent](#zonespec)</sup></sup>





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
          Name of the namespace.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>pool</b></td>
        <td>string</td>
        <td>
          Name of the Node Pool where to schedule workloads. If not specified, the first pool is used.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Zone.spec.pools[index]
<sup><sup>[↩ Parent](#zonespec)</sup></sup>





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
          Name of the node pool<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#zonespecpoolsindexrequirementsindex">requirements</a></b></td>
        <td>[]object</td>
        <td>
          Node pool requirements including instance type, capacity type, zone, and scaling limits<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### Zone.spec.pools[index].requirements[index]
<sup><sup>[↩ Parent](#zonespecpoolsindex)</sup></sup>





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
        <td><b>key</b></td>
        <td>enum</td>
        <td>
          Requirement key<br/>
          <br/>
            <i>Enum</i>: instance-type, capacity-type, zone, min-size, max-size, desired-size<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>value</b></td>
        <td>int or string</td>
        <td>
          Single value for capacity-type, min-size, max-size, or desired-size. Can be string or integer.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>values</b></td>
        <td>[]string</td>
        <td>
          Array of values for instance-type or availability-zone<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Zone.spec.appProject
<sup><sup>[↩ Parent](#zonespec)</sup></sup>





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
        <td><b>contributorGroups</b></td>
        <td>[]string</td>
        <td>
          OIDC groups with full access to applications<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>


</Tabs>
