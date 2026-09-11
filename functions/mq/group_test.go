package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

func withReadyStatus(jsonStr string) *fnv1.Resource {
	u := &unstructured.Unstructured{}
	if err := u.UnmarshalJSON([]byte(jsonStr)); err != nil {
		panic(err)
	}
	conditions := []interface{}{
		map[string]interface{}{"type": "Ready", "status": "True", "reason": "Available"},
		map[string]interface{}{"type": "Synced", "status": "True", "reason": "ReconcileSuccess"},
	}
	if err := unstructured.SetNestedSlice(u.Object, conditions, "status", "conditions"); err != nil {
		panic(fmt.Sprintf("failed set nested slice to unstructured: %v", err))
	}
	modifiedJSON, err := u.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("failed to marshal modified unstructured object back to JSON: %v", err))
	}
	return &fnv1.Resource{Resource: resource.MustStructJSON(string(modifiedJSON))}
}

func observedCredentialsSecretJSON(name, password string) string {
	pwB64 := base64.StdEncoding.EncodeToString([]byte(password))
	userB64 := base64.StdEncoding.EncodeToString([]byte("mqadmin"))
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"type":"Opaque","data":{"password":"%s","username":"%s"}}`,
		name, pwB64, userB64)
}

const (
	requiredResVPCjson = `{
		"apiVersion": "ec2.aws.m.upbound.io/v1beta1", "kind": "VPC",
		"metadata": {"annotations": {"crossplane.io/external-name": "vpc-01cda48a237c4850f"}, "name": "test-net-vpc", "namespace":"aws-provider"},
		"spec": {"forProvider": {"region": "eu-north-1"}}
	}`
	requiredMQSubnetGroupJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"SubnetGroup",
		"metadata":{"annotations":{"crossplane.io/external-name":"test-net-vpc"},"name":"test-net-vpc", "namespace":"aws-provider"},
		"status":{"atProvider":{"subnetIds":["subnet-aaa111","subnet-bbb222"]}}
	}`

	rabbitmqInputJson = `{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"configuration":{"data":"consumer_timeout = 1800000\n"},"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false}}`

	sgResJson             = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"description":"allow traffic from vpc","region":"eu-north-1","tags":{"Name":"%s","entigo:zone":"zone-a"},"vpcIdRef":{"name":"test-net-vpc","namespace":"aws-provider"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	ingressResJson        = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow amqps from vpc","fromPort":5671,"protocol":"tcp","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":5671,"type":"ingress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	consoleIngressResJson = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow management console from vpc","fromPort":443,"protocol":"tcp","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":443,"type":"ingress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	egressResJson         = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow traffic from vpc","fromPort":0,"protocol":"-1","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":0,"type":"egress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	credentialsResJson = `{"apiVersion":"v1","kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"type":"Opaque","stringData":{"password":"%s","username":"mqadmin"}}`

	// Configuration MR desired from spec.configuration.data (args: name, name).
	configResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Configuration","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"data":"consumer_timeout = 1800000\n","engineType":"RabbitMQ","engineVersion":"4.2","name":"%s","region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	// Observed Configuration MR carrying a latestRevision (args: name, name); wrap with withReadyStatus.
	configObservedResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Configuration","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"data":"consumer_timeout = 1800000\n","engineType":"RabbitMQ","engineVersion":"4.2","name":"%s","region":"eu-north-1","tags":{"entigo:zone":"zone-a"}}},"status":{"atProvider":{"latestRevision":1}}}`

	// New version-scoped Configuration MR desired after an engineVersion bump to 4.3 (args: name, name).
	configV43ResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Configuration","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"data":"consumer_timeout = 1800000\n","engineType":"RabbitMQ","engineVersion":"4.3","name":"%s","region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	// Old Configuration MR re-emitted (spec copied from observed) while the broker has not yet switched (args: name, name).
	staleConfigResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Configuration","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"data":"consumer_timeout = 1800000\n","engineType":"RabbitMQ","engineVersion":"4.2","name":"%s","region":"eu-north-1","tags":{"entigo:zone":"zone-a"}}}}`

	// Broker desired after an engineVersion bump to 4.3 associating the new Configuration (no revision yet observed).
	brokerV43ResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"applyImmediately":true,"autoMinorVersionUpgrade":true,"brokerName":"%s","configuration":{"idRef":{"name":"%s","namespace":"testspace","policy":{"resolve":"Always"}}},"deploymentMode":"SINGLE_INSTANCE","encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"engineType":"RabbitMQ","engineVersion":"4.3","hostInstanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false,"region":"eu-north-1","securityGroupRefs":[{"name":"%s"}],"subnetIds":["subnet-aaa111"],"tags":{"entigo:zone":"zone-a"},"user":[{"consoleAccess":true,"passwordSecretRef":{"key":"password","name":"%s"},"username":"mqadmin"}]},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s"}},"status":{"atProvider":{}}}`

	brokerResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"applyImmediately":true,"autoMinorVersionUpgrade":true,"brokerName":"%s","configuration":{"idRef":{"name":"%s","namespace":"testspace","policy":{"resolve":"Always"}},"revision":1},"deploymentMode":"SINGLE_INSTANCE","encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"engineType":"RabbitMQ","engineVersion":"4.2","hostInstanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false,"region":"eu-north-1","securityGroupRefs":[{"name":"%s"}],"subnetIds":["subnet-aaa111"],"tags":{"entigo:zone":"zone-a"},"user":[{"consoleAccess":true,"passwordSecretRef":{"key":"password","name":"%s"},"username":"mqadmin"}]},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s"}},"status":{"atProvider":{}}}`

	minimalBrokerResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"applyImmediately":true,"autoMinorVersionUpgrade":true,"brokerName":"%s","deploymentMode":"SINGLE_INSTANCE","encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"engineType":"RabbitMQ","engineVersion":"4.2","hostInstanceType":"mq.m7g.medium","publiclyAccessible":false,"region":"eu-north-1","securityGroupRefs":[{"name":"%s"}],"subnetIds":["subnet-aaa111"],"tags":{"entigo:zone":"zone-a"},"user":[{"consoleAccess":true,"passwordSecretRef":{"key":"password","name":"%s"},"username":"mqadmin"}]},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s"}},"status":{"atProvider":{}}}`

	multiAZBrokerResJson = `{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"applyImmediately":true,"autoMinorVersionUpgrade":true,"brokerName":"%s","deploymentMode":"ACTIVE_STANDBY_MULTI_AZ","encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"engineType":"RabbitMQ","engineVersion":"4.2","hostInstanceType":"mq.m7g.medium","publiclyAccessible":false,"region":"eu-north-1","securityGroupRefs":[{"name":"%s"}],"subnetIds":["subnet-aaa111","subnet-bbb222"],"tags":{"entigo:zone":"zone-a"},"user":[{"consoleAccess":true,"passwordSecretRef":{"key":"password","name":"%s"},"username":"mqadmin"}]},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s"}},"status":{"atProvider":{}}}`
)

func TestRabbitMQBrokerFunction(t *testing.T) {
	var cr v1alpha1.RabbitMQBroker
	if err := json.Unmarshal([]byte(rabbitmqInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}
	setHash := base.GenerateFNVHash(cr.UID)

	environmentData := map[string]interface{}{
		"awsProvider": "aws-provider",
		"dataKMSKey":  "data",
		"vpc":         "test-net-vpc",
		"subnetGroup": "test-net-vpc",
	}

	brokerCRName := "test-mq"
	sgName := service.GetSGName(brokerCRName, setHash)
	sgIngressName := service.GetSGIngressName(brokerCRName, setHash)
	sgConsoleIngressName := service.GetSGConsoleIngressName(brokerCRName, setHash)
	sgEgressName := service.GetSGEgressName(brokerCRName, setHash)
	brokerName := service.GetBrokerName(brokerCRName, setHash)
	configName := service.GetConfigurationName(brokerCRName, "4.2", setHash)
	oldConfigName := configName
	newConfigName := service.GetConfigurationName(brokerCRName, "4.3", setHash)
	credentialsSecretName := service.GetCredentialsSecretName(brokerCRName)
	connectionSecretName := service.GetConnectionSecretName(brokerCRName)
	ns := "testspace"
	reqResNs := "aws-provider"

	fixedPassword := "fixed-test-password-value-1234567"

	requiredResources := func() map[string]*fnv1.Resources {
		return map[string]*fnv1.Resources{
			"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
			"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
			"MQSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredMQSubnetGroupJson)}}},
		}
	}
	expectedRequirements := func() *fnv1.Requirements {
		return &fnv1.Requirements{
			Resources: map[string]*fnv1.ResourceSelector{
				"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
				"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
				"MQSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
			},
		}
	}

	cases := map[string]test.Case{
		"RabbitMQBroker/Stage 1: Create SecurityGroup and credentials when nothing observed": {
			Reason: "With all requirements met and no observed resources, the SecurityGroup stack and credentials secret are desired; the Broker is withheld until the first sequence group is Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(rabbitmqInputJson)},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName))},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, "IGNORED"))},
							configName:           {Resource: resource.MustStructJSON(fmt.Sprintf(configResJson, configName, configName))},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Stage 2: Create Broker when SecurityGroup and credentials are Ready": {
			Reason: "Once the SecurityGroup stack and credentials secret are Ready, the Broker is desired. Providing an observed credentials secret makes the reused password deterministic.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(rabbitmqInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
							configName:           withReadyStatus(fmt.Sprintf(configObservedResJson, configName, configName)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							configName:           {Resource: resource.MustStructJSON(fmt.Sprintf(configResJson, configName, configName)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(brokerResJson, brokerName, brokerName, configName, sgName, credentialsSecretName, connectionSecretName))},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Version bump: new versioned Configuration is created and the old one is kept until the broker switches": {
			Reason: "Amazon MQ Configuration engineVersion is immutable, so bumping spec.engineVersion creates a new version-scoped Configuration. The old Configuration is re-emitted (not deleted) until the broker reports it has switched to the new one's id.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"configuration":{"data":"consumer_timeout = 1800000\n"},"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.3","instanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
							oldConfigName:        withReadyStatus(fmt.Sprintf(configObservedResJson, oldConfigName, oldConfigName)),
							brokerName:           withReadyStatus(fmt.Sprintf(`{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{}},"status":{"atProvider":{"id":"b-1234","configuration":{"id":"c-old-config","revision":1}}}}`, brokerName)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							newConfigName:        {Resource: resource.MustStructJSON(fmt.Sprintf(configV43ResJson, newConfigName, newConfigName))},
							oldConfigName:        {Resource: resource.MustStructJSON(fmt.Sprintf(staleConfigResJson, oldConfigName, oldConfigName)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(brokerV43ResJson, brokerName, brokerName, newConfigName, sgName, credentialsSecretName, connectionSecretName)), Ready: 1},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Status: populate composite status from observed Broker": {
			Reason: "When the observed Broker reports status.atProvider, GetObservedStatus -> GetRabbitMQBrokerStatusFromBroker maps its scalar fields (id->amazonMQBrokerID, hostInstanceType->instanceType, engineType, engineVersion, deploymentMode, region, storageType, autoMinorVersionUpgrade, publiclyAccessible, pendingDataReplicationMode), the configuration/encryptionOptions/maintenanceWindowStartTime sub-objects, and the instances/securityGroups/subnetIds lists onto the RabbitMQBroker composite status.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"configuration":{"data":"consumer_timeout = 1800000\n"},"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false},"status":{}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
							configName:           withReadyStatus(fmt.Sprintf(configObservedResJson, configName, configName)),
							brokerName:           withReadyStatus(fmt.Sprintf(`{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{}},"status":{"atProvider":{"id":"b-1234abcd-5678-90ef","brokerName":"test-mq-broker-811c9dc5","autoMinorVersionUpgrade":true,"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","hostInstanceType":"mq.m7g.medium","pendingDataReplicationMode":"NONE","publiclyAccessible":false,"region":"eu-north-1","storageType":"EBS","configuration":{"id":"c-12345678","revision":1},"encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"instances":[{"consoleUrl":"https://console.example.com","endpoints":["amqps://b-1234.mq.eu-north-1.amazonaws.com:5671"],"ipAddress":"10.0.1.5"}],"securityGroups":["sg-abc123"],"subnetIds":["subnet-aaa111","subnet-bbb222"]}}}`, brokerName)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"configuration":{"data":"consumer_timeout = 1800000\n"},"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"publiclyAccessible":false},"status":{"amazonMQBrokerID":"b-1234abcd-5678-90ef","brokerName":"test-mq-broker-811c9dc5","autoMinorVersionUpgrade":true,"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","pendingDataReplicationMode":"NONE","publiclyAccessible":false,"region":"eu-north-1","storageType":"EBS","configuration":{"id":"c-12345678","revision":1},"encryptionOptions":{"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","useAwsOwnedKey":false},"maintenanceWindowStartTime":{"dayOfWeek":"MONDAY","timeOfDay":"02:00","timeZone":"CET"},"instances":[{"consoleUrl":"https://console.example.com","endpoints":["amqps://b-1234.mq.eu-north-1.amazonaws.com:5671"],"ipAddress":"10.0.1.5"}],"securityGroups":["sg-abc123"],"subnetIds":["subnet-aaa111","subnet-bbb222"]}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							configName:           {Resource: resource.MustStructJSON(fmt.Sprintf(configResJson, configName, configName)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(brokerResJson, brokerName, brokerName, configName, sgName, credentialsSecretName, connectionSecretName)), Ready: 1},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Minimal: broker without optional blocks omits configuration and maintenanceWindowStartTime": {
			Reason: "When the XR provides neither configuration nor maintenanceWindowStartTime, buildBroker must omit both blocks entirely (no null-valued fields that the provider CRD would reject).",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","publiclyAccessible":false}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(minimalBrokerResJson, brokerName, brokerName, sgName, credentialsSecretName, connectionSecretName))},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Multi-AZ: ACTIVE_STANDBY_MULTI_AZ uses all subnets": {
			Reason: "For multi-AZ deployment modes buildBroker must attach every available subnet from the subnet group, unlike SINGLE_INSTANCE which uses exactly one.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"deploymentMode":"ACTIVE_STANDBY_MULTI_AZ","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","publiclyAccessible":false}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(multiAZBrokerResJson, brokerName, brokerName, sgName, credentialsSecretName, connectionSecretName))},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
		"RabbitMQBroker/Status minimal: nil observation sub-blocks are omitted (no panic)": {
			Reason: "During broker creation the observed atProvider Configuration/EncryptionOptions/MaintenanceWindowStartTime pointers are nil; GetRabbitMQBrokerStatusFromBroker must nil-guard them (no panic) and omit those keys from the composite status while still mapping scalars and lists.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","publiclyAccessible":false},"status":{}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:        withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgConsoleIngressName: withReadyStatus(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)),
							sgEgressName:         withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							"credentials":        withReadyStatus(observedCredentialsSecretJSON(credentialsSecretName, fixedPassword)),
							brokerName:           withReadyStatus(fmt.Sprintf(`{"apiVersion":"mq.aws.m.upbound.io/v1beta1","kind":"Broker","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{}},"status":{"atProvider":{"id":"b-1234abcd-5678-90ef","brokerName":"test-mq-broker-811c9dc5","region":"eu-north-1","instances":[{"consoleUrl":"https://console.example.com","endpoints":["amqps://b-1234.mq.eu-north-1.amazonaws.com:5671"],"ipAddress":"10.0.1.5"}],"securityGroups":["sg-abc123"],"subnetIds":["subnet-aaa111"]}}}`, brokerName)),
						},
					},
					RequiredResources: requiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion":"mq.entigo.com/v1alpha1","kind":"RabbitMQBroker","metadata":{"name":"test-mq","namespace":"testspace"},"spec":{"autoMinorVersionUpgrade":true,"deploymentMode":"SINGLE_INSTANCE","engineType":"RabbitMQ","engineVersion":"4.2","instanceType":"mq.m7g.medium","publiclyAccessible":false},"status":{"amazonMQBrokerID":"b-1234abcd-5678-90ef","brokerName":"test-mq-broker-811c9dc5","region":"eu-north-1","instances":[{"consoleUrl":"https://console.example.com","endpoints":["amqps://b-1234.mq.eu-north-1.amazonaws.com:5671"],"ipAddress":"10.0.1.5"}],"securityGroups":["sg-abc123"],"subnetIds":["subnet-aaa111"]}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:               {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgConsoleIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(consoleIngressResJson, sgConsoleIngressName, sgName)), Ready: 1},
							sgEgressName:         {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							"credentials":        {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, credentialsSecretName, fixedPassword)), Ready: 1},
							brokerName:           {Resource: resource.MustStructJSON(fmt.Sprintf(minimalBrokerResJson, brokerName, brokerName, sgName, credentialsSecretName, connectionSecretName)), Ready: 1},
						},
					},
					Requirements: expectedRequirements(),
				},
			},
		},
	}

	test.AddEnvironmentConfig(cases, environmentName, environmentData)
	test.AddZoneResources(cases, ns, "zone-a")
	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases, "annotations", "force-sync", "lastTransitionTime", "password")
}
