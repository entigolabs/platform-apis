import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# ValkeyInstance

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [database.entigo.com/v1alpha1](#database.entigo.com/v1alpha1)

# database.entigo.com/v1alpha1

Resource Types:

- [ValkeyInstance](#valkeyinstance)




## ValkeyInstance
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
      <td>ValkeyInstance</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#valkeyinstancespec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#valkeyinstancestatus">status</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### ValkeyInstance.spec
<sup><sup>[↩ Parent](#valkeyinstance)</sup></sup>





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
        <td><b>engineVersion</b></td>
        <td>string</td>
        <td>
          Cache engine version. ElastiCache automatically uses the latest patch version.
For supported versions, see: https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/supported-engine-versions.html
<br/>
          <br/>
            <i>Default</i>: 8.2<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>instanceType</b></td>
        <td>string</td>
        <td>
          ElastiCache instance type determines memory, compute, and network capacity.
For a list of available instance types, see: https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/CacheNodes.SupportedTypes.html
<br/>
          <br/>
            <i>Default</i>: cache.t4g.small<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>autoMinorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          Indicates that minor engine upgrades will be applied automatically during the maintenance window.
<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>deletionProtection</b></td>
        <td>boolean</td>
        <td>
          Deletion protection for the ElastiCache replication group.
<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>maintenanceWindow</b></td>
        <td>string</td>
        <td>
          Weekly time range during which system maintenance can occur (in UTC).
Format: ddd:hh24:mi-ddd:hh24:mi. Example: "sun:05:00-sun:06:00"
Must be at least 60 minutes.
<br/>
          <br/>
            <i>Default</i>: sun:05:00-sun:06:00<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>numCacheClusters</b></td>
        <td>integer</td>
        <td>
          Number of cache clusters in the replication group (1 primary + replicas).
Minimum 2 required (1 primary + 1 replica). Maximum 6 (1 primary + 5 replicas).
<br/>
          <br/>
            <i>Default</i>: 2<br/>
            <i>Minimum</i>: 2<br/>
            <i>Maximum</i>: 6<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>parameterGroupName</b></td>
        <td>string</td>
        <td>
          Name of the parameter group to associate with this replication group.
If not specified, the default parameter group for the engine version is used.
<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>snapshotRetentionLimit</b></td>
        <td>integer</td>
        <td>
          Number of days for which ElastiCache retains automatic snapshots before deleting them.
Set to 0 to disable automated backups.
<br/>
          <br/>
            <i>Default</i>: 7<br/>
            <i>Minimum</i>: 0<br/>
            <i>Maximum</i>: 35<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>snapshotWindow</b></td>
        <td>string</td>
        <td>
          Daily time range (in UTC) during which ElastiCache begins taking daily snapshots.
Format: hh24:mi-hh24:mi. Example: "03:00-05:00"
Must not overlap with maintenance_window.
<br/>
          <br/>
            <i>Default</i>: 03:00-05:00<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### ValkeyInstance.status
<sup><sup>[↩ Parent](#valkeyinstance)</sup></sup>





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
        <td><b>autoMinorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#valkeyinstancestatusendpoint">endpoint</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>kmsKeyAlias</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>kmsKeyId</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>multiAZenabled</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>parameterGroupName</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#valkeyinstancestatussecuritygroup">securityGroup</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### ValkeyInstance.status.endpoint
<sup><sup>[↩ Parent](#valkeyinstancestatus)</sup></sup>





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
        <td><b>address</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>port</b></td>
        <td>number</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### ValkeyInstance.status.securityGroup
<sup><sup>[↩ Parent](#valkeyinstancestatus)</sup></sup>





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
        <td><b>arn</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#valkeyinstancestatussecuritygrouprulesindex">rules</a></b></td>
        <td>[]object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### ValkeyInstance.status.securityGroup.rules[index]
<sup><sup>[↩ Parent](#valkeyinstancestatussecuritygroup)</sup></sup>





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
        <td><b>cidrBlocks</b></td>
        <td>[]string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>fromPort</b></td>
        <td>integer</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>protocol</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>self</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>toPort</b></td>
        <td>integer</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>


</Tabs>
