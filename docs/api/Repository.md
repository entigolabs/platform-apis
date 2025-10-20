import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# Repository

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [artifact.entigo.com/v1alpha1](#artifact.entigo.com/v1alpha1)

# artifact.entigo.com/v1alpha1

Resource Types:

- [Repository](#repository)




## Repository
<sup><sup>[↩ Parent](#artifact.entigo.com/v1alpha1 )</sup></sup>








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
      <td>artifact.entigo.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>Repository</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#repositoryspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#repositorystatus">status</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Repository.spec
<sup><sup>[↩ Parent](#repository)</sup></sup>





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
          Name of the repository, defaults to the name of the resource if not specified<br/>
          <br/>
            <i>Validations</i>:<li>self == oldSelf: Value is immutable</li>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>path</b></td>
        <td>string</td>
        <td>
          Prefix path for the repository<br/>
          <br/>
            <i>Validations</i>:<li>self == oldSelf: Value is immutable</li>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Repository.status
<sup><sup>[↩ Parent](#repository)</sup></sup>





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
        <td><b>repositoryUri</b></td>
        <td>string</td>
        <td>
          Repository URI for pulling and pushing images.<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>


</Tabs>
