# PostgreSQLInstance

Packages:

- [database.entigo.com/v1alpha1](#database.entigo.com/v1alpha1)

# database.entigo.com/v1alpha1

Resource Types:

- [PostgreSQLInstance](#postgresqlinstance)




## PostgreSQLInstance
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
      <td>PostgreSQLInstance</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#postgresqlinstancespec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#postgresqlinstancestatus">status</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLInstance.spec
<sup><sup>[↩ Parent](#postgresqlinstance)</sup></sup>





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
        <td><b>allocatedStorage</b></td>
        <td>number</td>
        <td>
          Database disk size in GB. Default is 20GB.<br/>
          <br/>
            <i>Default</i>: 20<br/>
            <i>Minimum</i>: 20<br/>
            <i>Maximum</i>: 5000<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>engineVersion</b></td>
        <td>string</td>
        <td>
          Database engine version. Use AWS documentation to find a list of supported versions: https://docs.aws.amazon.com/AmazonRDS/latest/PostgreSQLReleaseNotes/postgresql-versions.html
<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>instanceType</b></td>
        <td>string</td>
        <td>
          AWS database instance type. For a list of available instance types: https://aws.amazon.com/rds/instance-types/
<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>allowMajorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          Indicates that major version upgrades are allowed. Default is false.<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>autoMinorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          Indicates that minor engine upgrades will be applied automatically to the DB instance during the maintenance window. Defaults to true.<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>backupWindow</b></td>
        <td>string</td>
        <td>
          The daily time range (in UTC) during which automated backups are created if they are enabled. Example: '09:46-10:16'. 
Must not overlap with maintenance_window. Default value is determined by the region.
<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>deletionProtection</b></td>
        <td>boolean</td>
        <td>
          If the DB instance should have deletion protection enabled. The database can't be deleted when this value is set to true. The default is true.<br/>
          <br/>
            <i>Default</i>: true<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>iops</b></td>
        <td>number</td>
        <td>
          Use reserved IOPS for the database. For most use-cases it is more cost effective to increase the disk size than to by IOPS. Default unset.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>maintenanceWindow</b></td>
        <td>string</td>
        <td>
          The window to perform maintenance in. Syntax: "ddd:hh24:mi-ddd:hh24:mi". Eg: "Mon:00:00-Mon:03:00". See RDS Maintenance Window docs for more information.
<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>multiAZ</b></td>
        <td>boolean</td>
        <td>
          Specifies if the Database instance is multi-AZ. Default false.<br/>
          <br/>
            <i>Default</i>: false<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>parameterGroupName</b></td>
        <td>string</td>
        <td>
          Reference to a custom parameter group.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLInstance.status
<sup><sup>[↩ Parent](#postgresqlinstance)</sup></sup>





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
        <td><b>allowMajorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>autoMinorVersionUpgrade</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>backupWindow</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>dbInstanceIdentifier</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#postgresqlinstancestatusendpoint">endpoint</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>finalSnapshotIdentifier</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>iops</b></td>
        <td>number</td>
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
        <td><b>latestRestorableTime</b></td>
        <td>string</td>
        <td>
          <br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>maintenanceWindow</b></td>
        <td>string</td>
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
        <td><b>resourceId</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>storageEncrypted</b></td>
        <td>boolean</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>storageThroughput</b></td>
        <td>number</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>storageType</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>vpcSecurityGroupIds</b></td>
        <td>[]string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### PostgreSQLInstance.status.endpoint
<sup><sup>[↩ Parent](#postgresqlinstancestatus)</sup></sup>





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
        <td><b>hostedZoneId</b></td>
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