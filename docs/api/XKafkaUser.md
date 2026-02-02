import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

<Tabs queryString="tab">

<TabItem value="api-reference" label="API Reference" default>

# XKafkaUser

:::note Tiers
This feature is available for the following tiers: **Standard, Premium**.
:::


Packages:

- [kafka.entigo.com/v1alpha1](#kafka.entigo.com/v1alpha1)

# kafka.entigo.com/v1alpha1

Resource Types:

- [XKafkaUser](#xkafkauser)




## XKafkaUser
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
      <td>XKafkaUser</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#xkafkauserspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### XKafkaUser.spec
<sup><sup>[↩ Parent](#xkafkauser)</sup></sup>





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
          <br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#xkafkauserspecaclsindex">acls</a></b></td>
        <td>[]object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>consumerGroups</b></td>
        <td>[]string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### XKafkaUser.spec.acls[index]
<sup><sup>[↩ Parent](#xkafkauserspec)</sup></sup>





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
        <td><b>operation</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>topic</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>

</TabItem>

<TabItem value="examples" label="Examples">
### Basic Kafka user {#example-basic-kafka-user}

```yaml
---
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-a
  namespace: default
spec:
  clusterName: test-crossplane-cluster
```

### Kafka user with Consumer Group ACL's {#example-kafka-user-with-consumer-group-acl-s}

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-b
  namespace: team-b
spec:
  clusterName: test-crossplane-cluster
  consumerGroups:
    - alpha
    - gamma
```

### Kafka user with Topic ACL's {#example-kafka-user-with-topic-acl-s}

```yaml
apiVersion: kafka.entigo.com/v1alpha1
kind: KafkaUser
metadata:
  name: user-b
  namespace: team-b
spec:
  clusterName: test-crossplane-cluster
  acls:
    - topic: topic-a
      operation: Read
    - topic: topic-b
      operation: Write
```
</TabItem>

</Tabs>
