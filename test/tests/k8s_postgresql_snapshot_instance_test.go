package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	awsrds "github.com/aws/aws-sdk-go/service/rds"
	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlSnapshotInstanceName = "postgresql-instance-from-snapshot-test"
)

func runPostgresqlSnapshotInstanceTests(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	snapshotID := testFindAutomatedSnapshot(t, namespaceOptions)

	testPostgresqlSnapshotInstanceApplied(t, namespaceOptions, snapshotID)
	t.Run("snapshot-sub-resources", func(t *testing.T) {
		t.Run("rds-snapshot-instance", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, namespaceOptions, RdsInstanceKind, PostgresqlSnapshotInstanceName, 90, 10*time.Second)
		})
	})

	if t.Failed() {
		return
	}
	testPostgresqlSnapshotInstanceSyncedAndReady(t, namespaceOptions)
	testSnapshotInstanceVerifiedFromSnapshot(t, namespaceOptions, snapshotID)
	testSnapshotIdentifierImmutable(t, namespaceOptions)
}

func testFindAutomatedSnapshot(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) string {
	rdsName, err := getFirstByLabel(t, namespaceOptions, RdsInstanceKind, PostgresqlInstanceName)
	require.NoError(t, err, "failed to find RDS instance for %s", PostgresqlInstanceName)
	require.NotEmpty(t, rdsName, "no RDS instance found for %s", PostgresqlInstanceName)

	dbIdentifier, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.identifier}")
	require.NoError(t, err, "failed to get RDS instance identifier")
	require.NotEmpty(t, dbIdentifier, "RDS instance identifier is empty")

	region, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.region}")
	require.NoError(t, err, "failed to get RDS instance region")
	require.NotEmpty(t, region, "RDS instance region is empty")

	var snapshotID string
	_, err = retry.DoWithRetryE(t, fmt.Sprintf("waiting for automated snapshot for %s", dbIdentifier), 60, 2*time.Minute, func() (string, error) {
		id, findErr := findLatestAutomatedSnapshot(dbIdentifier, region)
		if findErr != nil {
			return "", findErr
		}
		snapshotID = id
		return id, nil
	})
	require.NoError(t, err, "no automated snapshot found for RDS instance %s", dbIdentifier)
	return snapshotID
}

func findLatestAutomatedSnapshot(dbIdentifier, region string) (string, error) {
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		return "", fmt.Errorf("failed to create AWS session: %w", err)
	}
	svc := awsrds.New(sess)
	resp, err := svc.DescribeDBSnapshots(&awsrds.DescribeDBSnapshotsInput{
		DBInstanceIdentifier: aws.String(dbIdentifier),
		SnapshotType:         aws.String("automated"),
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe RDS snapshots for %s: %w", dbIdentifier, err)
	}
	var latest *awsrds.DBSnapshot
	for _, s := range resp.DBSnapshots {
		if s.Status == nil || *s.Status != "available" || s.DBSnapshotIdentifier == nil {
			continue
		}
		if latest == nil || (s.SnapshotCreateTime != nil && latest.SnapshotCreateTime != nil && s.SnapshotCreateTime.After(*latest.SnapshotCreateTime)) {
			latest = s
		}
	}
	if latest == nil {
		return "", fmt.Errorf("no available automated snapshots for DB instance %s", dbIdentifier)
	}
	return *latest.DBSnapshotIdentifier, nil
}

func testPostgresqlSnapshotInstanceApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions, snapshotID string) {
	yaml := fmt.Sprintf(`apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: %s
  namespace: %s
spec:
  allocatedStorage: 20
  engineVersion: "17.2"
  instanceType: db.t3.micro
  allowMajorVersionUpgrade: false
  autoMinorVersionUpgrade: true
  deletionProtection: false
  multiAZ: false
  snapshotIdentifier: %s
`, PostgresqlSnapshotInstanceName, PostgresqlNamespaceName, snapshotID)

	tmpFile, err := os.CreateTemp("", "snapshot-instance-*.yaml")
	require.NoError(t, err, "failed to create temp file for snapshot instance")
	defer os.Remove(tmpFile.Name())

	_, err = fmt.Fprint(tmpFile, yaml)
	require.NoError(t, err, "failed to write snapshot instance YAML")
	require.NoError(t, tmpFile.Close())

	_, err = terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", tmpFile.Name())
	require.NoError(t, err, "applying PostgreSQL snapshot instance error")
}

func testPostgresqlSnapshotInstanceSyncedAndReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	waitSyncedAndReady(t, namespaceOptions, PostgresqlInstanceKind, PostgresqlSnapshotInstanceName, 90, 10*time.Second)
}

func testSnapshotInstanceVerifiedFromSnapshot(t *testing.T, namespaceOptions *terrak8s.KubectlOptions, expectedSnapshotID string) {
	rdsName, err := getFirstByLabel(t, namespaceOptions, RdsInstanceKind, PostgresqlSnapshotInstanceName)
	require.NoError(t, err, "failed to find RDS instance for %s", PostgresqlSnapshotInstanceName)
	require.NotEmpty(t, rdsName, "no RDS instance found for %s", PostgresqlSnapshotInstanceName)

	snapshotID, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.snapshotIdentifier}")
	require.NoError(t, err, "failed to get snapshotIdentifier from RDS instance")
	require.Equal(t, expectedSnapshotID, snapshotID, "RDS instance snapshotIdentifier does not match the expected snapshot")
}

func testSnapshotIdentifierImmutable(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	output, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "patch", PostgresqlInstanceKind, PostgresqlSnapshotInstanceName,
		"-n", PostgresqlNamespaceName, "--type", "merge", "-p", `{"spec":{"snapshotIdentifier":"rds:different-snapshot-id"}}`)
	require.Error(t, err, "patching snapshotIdentifier to a different value should be rejected")
	require.Contains(t, output, "snapshotIdentifier is immutable", "rejection message should mention immutability")
}
