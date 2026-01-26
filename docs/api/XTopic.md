import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# XTopic

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [kafka.entigo.com/v1alpha1](#kafka.entigo.com/v1alpha1)

# kafka.entigo.com/v1alpha1

Resource Types:

- [XTopic](#xtopic)




## XTopic
<sup><sup>[↩ Parent](#kafka.entigo.com/v1alpha1 )</sup></sup>








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
      <td>kafka.entigo.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>XTopic</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#xtopicspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### XTopic.spec
<sup><sup>[↩ Parent](#xtopic)</sup></sup>





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
        <td><b>clusterName</b></td>
        <td>string</td>
        <td>
          Name of the MSK cluster name<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>config</b></td>
        <td>map[string]string</td>
        <td>
          Topic-level configuration<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>partitions</b></td>
        <td>integer</td>
        <td>
          Number of partitions<br/>
          <br/>
            <i>Default</i>: 3<br/>
            <i>Minimum</i>: 1<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>replicationFactor</b></td>
        <td>integer</td>
        <td>
          Replication factor<br/>
          <br/>
            <i>Default</i>: 3<br/>
            <i>Minimum</i>: 1<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>

<TabItem value="examples" label="Examples">
### Basic Kafka Topic {#example-basic-kafka-topic}

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: Topic
metadata:
  name: topic-a
  namespace: team-a
spec:
  clusterName: test-crossplane-cluster
  partitions: 1
  replicationFactor: 1
  config:
    retention.ms: "604800012"
    ...
```
</TabItem>

</Tabs>
