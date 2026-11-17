"""Install the draft schemas as CRDs on a Kubernetes API server and test their validation rules.

Usage: python3 test_rules.py <schemas directory> <kubeconfig>

Any empty API server will do, such as kube-apiserver and etcd from setup-envtest. Each case creates
or updates one object and expects it to be accepted, or rejected with a given message.
"""
import copy
import json
import subprocess
import sys
from pathlib import Path

import yaml

SCHEMAS = Path(sys.argv[1])
KUBECONFIG = sys.argv[2]
NS = "default"


def kubectl(*args, stdin=None):
    r = subprocess.run(["kubectl", "--kubeconfig", KUBECONFIG, *args], input=stdin,
                       capture_output=True, text=True)
    return r.returncode, (r.stdout + r.stderr).strip()


def install():
    groups = {}
    for f in sorted(SCHEMAS.glob("*.yaml")):
        x = yaml.safe_load(f.read_text())
        s = x["spec"]
        v = s["versions"][0]
        if x["kind"] == "CustomResourceDefinition":
            code, out = kubectl("apply", "-f", "-", stdin=json.dumps(x))
            print(("INSTALLED " if code == 0 else "INSTALL FAILED ") + f.name + ("" if code == 0 else "\n" + out))
            groups[s["names"]["kind"]] = (s["group"] + "/" + v["name"], s["scope"])
            continue
        crd = {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
               "metadata": {"name": x["metadata"]["name"]},
               "spec": {"group": s["group"], "scope": s["scope"],
                        "names": {"kind": s["names"]["kind"], "plural": s["names"]["plural"]},
                        "versions": [{"name": v["name"], "served": True, "storage": True,
                                      "schema": v["schema"]}]}}
        code, out = kubectl("apply", "-f", "-", stdin=json.dumps(crd))
        print(("INSTALLED " if code == 0 else "INSTALL FAILED ") + f.name + ("" if code == 0 else "\n" + out))
        groups[s["names"]["kind"]] = (s["group"] + "/" + v["name"], s["scope"])
    kubectl("wait", "--for=condition=Established", "crd", "--all", "--timeout=60s")
    return groups


GV = install()
results = []
n = 0


def obj(kind, spec):
    global n
    n += 1
    gv, scope = GV[kind]
    meta = {"name": f"t{n}"} | ({"namespace": NS} if scope == "Namespaced" else {})
    return {"apiVersion": gv, "kind": kind, "metadata": meta, "spec": spec}


def check(label, code, out, expect_ok, expect_msg=None):
    ok = (code == 0) == expect_ok and (expect_ok or not expect_msg or expect_msg in out)
    results.append(ok)
    print(("PASS " if ok else "FAIL ") + label + ("" if ok else "\n     " + out[-400:]))


def create(label, kind, spec, expect_ok=True, msg=None):
    o = obj(kind, spec)
    code, out = kubectl("create", "-f", "-", stdin=json.dumps(o))
    check(label, code, out, expect_ok, msg)
    return o


def update(label, kind, spec, new_spec, expect_ok=True, msg=None):
    o = obj(kind, spec)
    code, out = kubectl("create", "-f", "-", stdin=json.dumps(o))
    if code != 0:
        results.append(False)
        print("FAIL " + label + " (create)\n     " + out)
        return
    o2 = copy.deepcopy(o)
    o2["spec"] = new_spec
    code, out = kubectl("replace", "-f", "-", stdin=json.dumps(o2))
    check(label, code, out, expect_ok, msg)


RDS = {"apiVersion": "aws.database.entigo.com/v1alpha1", "kind": "PostgreSQLRDSConfig",
       "spec": {"instanceType": "db.t4g.small"}}
SQL = {"apiVersion": "gcp.database.entigo.com/v1alpha1", "kind": "PostgreSQLCloudSQLConfig",
       "spec": {"instanceType": "db-custom-2-7680", "edition": "ENTERPRISE"}}
PG = {"version": "17", "storage": {"sizeGB": 20, "autoGrow": True, "maxSizeGB": 100},
      "highAvailability": False, "deletionProtection": True,
      "maintenanceWindow": {"day": "Sun", "hourUTC": 3}, "autoMinorVersionUpgrade": True,
      "allowMajorVersionUpgrade": False, "backup": {"retentionDays": 7, "startHourUTC": 2},
      "parameters": {"max_connections": "200"}, "providerConfigs": [RDS, SQL]}

def w(base, **kw):
    s = copy.deepcopy(base)
    for k, v in kw.items():
        if v is None:
            s.pop(k, None)
        else:
            s[k] = v
    return s


print("\n# PostgreSQLCluster")
create("valid spec", "PostgreSQLCluster", PG)
create("className accepted", "PostgreSQLCluster", w(PG, className="standard"))
update("changing className rejected", "PostgreSQLCluster", w(PG, className="standard"),
       w(PG, className="in-cluster"), False, "className is immutable")
create("two entries of one kind rejected", "PostgreSQLCluster", w(PG, providerConfigs=[RDS, RDS]),
       False, "Duplicate value")
create("an entry with ref accepted", "PostgreSQLCluster",
       w(PG, providerConfigs=[{**{k: RDS[k] for k in ("apiVersion", "kind")}, "ref": {"name": "rds"}}]))
create("an entry with both spec and ref rejected", "PostgreSQLCluster",
       w(PG, providerConfigs=[{**RDS, "ref": {"name": "rds"}}]), False, "either spec or ref")
create("an entry with neither spec nor ref rejected", "PostgreSQLCluster",
       w(PG, providerConfigs=[{k: RDS[k] for k in ("apiVersion", "kind")}]), False, "either spec or ref")
create("an entry of an unknown kind accepted, opaque to the core (rule 10)", "PostgreSQLCluster",
       w(PG, providerConfigs=[{"apiVersion": "lab.example.com/v1", "kind": "PostgreSQLLabConfig",
                               "spec": {"anything": 1}}]))
create("instanceType at the top level rejected", "PostgreSQLCluster",
       w(PG, instanceType="db.t4g.small"), False, "unknown field")
create("readReplicas accepted", "PostgreSQLCluster", w(PG, readReplicas=2))
create("negative readReplicas rejected", "PostgreSQLCluster", w(PG, readReplicas=-1), False,
       "greater than or equal to 0")
create("disasterRecovery accepted", "PostgreSQLCluster",
       w(PG, disasterRecovery={"workspaces": ["prod", "prod-dr"], "primary": "prod"}))
create("disasterRecovery primary outside workspaces rejected", "PostgreSQLCluster",
       w(PG, disasterRecovery={"workspaces": ["prod", "prod-dr"], "primary": "dev"}), False,
       "primary must be one of workspaces")
create("disasterRecovery with one workspace rejected", "PostgreSQLCluster",
       w(PG, disasterRecovery={"workspaces": ["prod"], "primary": "prod"}), False,
       "should have at least 2 items")
create("observe with an entry accepted", "PostgreSQLCluster",
       {"observe": True, "providerConfigs": [{**RDS, "spec": {"id": "db-ABC"}}]})
create("observe with a common field rejected", "PostgreSQLCluster",
       {"observe": True, "version": "17", "providerConfigs": [RDS]}, False, "no other common field")
ob = {"observe": True, "providerConfigs": [{**RDS, "spec": {"id": "db-ABC"}}]}
update("removing observe rejected", "PostgreSQLCluster", ob, {"providerConfigs": ob["providerConfigs"]},
       False, "observe is immutable")
create("maxSizeGB below sizeGB rejected", "PostgreSQLCluster",
       w(PG, storage={"sizeGB": 100, "maxSizeGB": 50}), False, "maxSizeGB must be at least sizeGB")
create("pointInTimeDays with retentionDays 0 rejected", "PostgreSQLCluster",
       w(PG, backup={"retentionDays": 0, "pointInTimeDays": 3}), False, "needs backups")
create("retentionDays -1 rejected", "PostgreSQLCluster", w(PG, backup={"retentionDays": -1}), False,
       "greater than or equal to 0")
create("minor version with minor upgrades on rejected", "PostgreSQLCluster", w(PG, version="17.2"),
       False, "takes a minor exactly")
pin = w(PG, version="17.7", autoMinorVersionUpgrade=False)
create("full version with minor upgrades off accepted", "PostgreSQLCluster", pin)
update("minor change of a pinned version accepted", "PostgreSQLCluster", pin, w(pin, version="17.8"))
update("major version change rejected", "PostgreSQLCluster", PG, w(PG, version="18"), False,
       "needs allowMajorVersionUpgrade")
update("major version change with allowMajorVersionUpgrade accepted", "PostgreSQLCluster",
       w(PG, allowMajorVersionUpgrade=True), w(PG, version="18", allowMajorVersionUpgrade=True))
update("raising storage.sizeGB accepted", "PostgreSQLCluster", PG,
       w(PG, storage={"sizeGB": 40, "autoGrow": True, "maxSizeGB": 100}))
update("lowering storage.sizeGB rejected", "PostgreSQLCluster", PG,
       w(PG, storage={"sizeGB": 10, "autoGrow": True, "maxSizeGB": 100}), False, "cannot decrease")
create("restore without fromInstance rejected", "PostgreSQLCluster",
       w(PG, restore={"pointInTime": "2026-11-01T10:00:00Z"}), False, "Required value")
rs = w(PG, restore={"fromInstance": "orders-db-old"})
update("adding restore rejected", "PostgreSQLCluster", PG, rs, False, "restore is set")
update("changing restore rejected", "PostgreSQLCluster", rs, w(rs, restore={"fromInstance": "x"}),
       False, "restore is set")
create("parametersRef accepted", "PostgreSQLCluster", w(PG, parametersRef={"name": "orders-tuning"}))
create("unknown parameterApplyMethod rejected", "PostgreSQLCluster",
       w(PG, parameterApplyMethod="Later"), False, "Unsupported value")
create("secretName accepted", "PostgreSQLCluster", w(PG, secretName="orders-db-admin"))

print("\n# PostgreSQLClass")
CLS = {"template": {"version": "17", "storage": {"sizeGB": 50}, "backup": {"retentionDays": 14},
                    "providerConfig": RDS,
                    "parametersRef": {"name": "pg-baseline", "namespace": "platform-apis"}},
       "enforced": {"deletionProtection": True,
                    "providerConfig": {**RDS, "spec": {"instanceType": "db.t4g.medium"}}}}
create("valid class", "PostgreSQLClass", CLS)
create("template and enforced naming different kinds rejected", "PostgreSQLClass",
       w(CLS, enforced={"providerConfig": SQL}), False, "the same provider config kind")
create("a class with only common fields accepted", "PostgreSQLClass",
       {"template": {"version": "17"}, "enforced": {"deletionProtection": True}})
create("a standby accepted", "PostgreSQLClass", w(CLS, standby={"providerConfig": RDS}))
create("a class without parametersRef namespace rejected", "PostgreSQLClass",
       {"template": {"parametersRef": {"name": "x"}}}, False, "Required value")
create("disasterRecovery in a class rejected", "PostgreSQLClass",
       {"template": {"disasterRecovery": {"workspaces": ["a", "b"], "primary": "a"}}}, False,
       "unknown field")

print("\n# PostgreSQLParameters and provider configs")
create("parameters accepted", "PostgreSQLParameters", {"parameters": {"work_mem": "64MB"}})
create("parameters without the map rejected", "PostgreSQLParameters", {}, False, "Required value")
for kind, spec in {"PostgreSQLRDSConfig": {"instanceType": "db.m7g.large",
                                           "readReplicas": [{"name": "reporting",
                                                             "instanceType": "db.r7g.2xlarge"}]},
                   "PostgreSQLCloudSQLConfig": {"instanceType": "db-custom-2-7680",
                                                "edition": "ENTERPRISE", "diskType": "PD_SSD"},
                   "PostgreSQLOCIConfig": {"instanceType": "PostgreSQL.VM.Standard.E5.Flex",
                                           "ocpus": 2, "memoryGB": 32},
                   "PostgreSQLFlexibleServerConfig": {"instanceType": "GP_Standard_D2ds_v5",
                                                      "storageTier": "P10"}}.items():
    create(f"{kind} valid spec", kind, spec)
    create(f"{kind} unknown field rejected", kind, {**spec, "unknownSetting": 1}, False, "unknown field")
    update(f"{kind} changing id rejected", kind, {**spec, "id": "a"}, {**spec, "id": "b"}, False,
           "id is immutable")
update("PostgreSQLRDSConfig changing fromBackup rejected", "PostgreSQLRDSConfig",
       {"fromBackup": "rds:a"}, {"fromBackup": "rds:b"}, False, "set at creation only")

print("\n# MariaDBServer")
MRDS = {"apiVersion": "aws.database.entigo.com/v1alpha1", "kind": "MariaDBRDSConfig",
        "spec": {"instanceType": "db.t4g.small"}}
MA = {"version": "11.4", "storage": {"sizeGB": 20}, "providerConfigs": [MRDS]}
create("MariaDB valid spec", "MariaDBServer", MA)
create("MariaDB one-number version rejected", "MariaDBServer", w(MA, version="11"), False,
       "should match")
MP = w(MA, version="11.4.10", autoMinorVersionUpgrade=False)
create("MariaDB full version with minor upgrades off accepted", "MariaDBServer", MP)
create("MariaDB full version with minor upgrades on rejected", "MariaDBServer",
       w(MA, version="11.4.10"), False, "takes a minor exactly")
create("MariaDB major only with minor upgrades off rejected", "MariaDBServer",
       w(MP, version="11.4"), False, "takes a minor exactly")
update("MariaDB minor change accepted", "MariaDBServer", MP, w(MP, version="11.4.11"))
update("MariaDB major change without allowMajorVersionUpgrade rejected", "MariaDBServer", MA,
       w(MA, version="11.8"), False, "allowMajorVersionUpgrade")
update("MariaDB major change with allowMajorVersionUpgrade accepted", "MariaDBServer",
       w(MA, allowMajorVersionUpgrade=True), w(MA, version="11.8", allowMajorVersionUpgrade=True))
update("MariaDB lowering storage.sizeGB rejected", "MariaDBServer", MA,
       w(MA, storage={"sizeGB": 10}), False, "cannot decrease")
create("MariaDB maxSizeGB below sizeGB rejected", "MariaDBServer",
       w(MA, storage={"sizeGB": 100, "maxSizeGB": 50}), False, "maxSizeGB must be at least sizeGB")
create("MariaDB readReplicas accepted", "MariaDBServer", w(MA, readReplicas=2))
create("MariaDB disasterRecovery accepted", "MariaDBServer",
       w(MA, disasterRecovery={"workspaces": ["prod", "prod-dr"], "primary": "prod"}))
create("MariaDB observe with an entry accepted", "MariaDBServer",
       {"observe": True, "providerConfigs": [{**MRDS, "spec": {"id": "db-ABC"}}]})
create("MariaDB pointInTime without fromInstance rejected", "MariaDBServer",
       w(MA, restore={"pointInTime": "2026-11-01T10:00:00Z"}), False, "Required value")
update("MariaDB adding restore later rejected", "MariaDBServer", MA,
       w(MA, restore={"fromInstance": "a"}), False, "restore is set at creation only")
create("MariaDBClass valid class", "MariaDBClass",
       {"template": {"version": "11.4", "providerConfig": MRDS}, "enforced": {"deletionProtection": True}})
create("MariaDBClass disasterRecovery rejected", "MariaDBClass",
       {"template": {"disasterRecovery": {"workspaces": ["a", "b"], "primary": "a"}}}, False,
       "unknown field")
create("MariaDBParameters accepted", "MariaDBParameters", {"parameters": {"long_query_time": "2"}})
MRS = {"instanceType": "db.t4g.small", "parameterGroupName": "orders-mariadb114",
       "readReplicas": [{"name": "reporting", "instanceType": "db.r7g.2xlarge"}]}
create("MariaDBRDSConfig valid spec", "MariaDBRDSConfig", MRS)
create("MariaDBRDSConfig unknown field rejected", "MariaDBRDSConfig", {**MRS, "unknownSetting": 1},
       False, "unknown field")
update("MariaDBRDSConfig changing fromBackup rejected", "MariaDBRDSConfig",
       {"fromBackup": "a"}, {"fromBackup": "b"}, False, "set at creation only")

print("\n# ValkeyServer")
EC = {"apiVersion": "aws.database.entigo.com/v1alpha1", "kind": "ValkeyElastiCacheConfig",
      "spec": {"instanceType": "cache.t4g.small"}}
OCC = {"apiVersion": "oci.database.entigo.com/v1alpha1", "kind": "ValkeyOCICacheConfig",
       "spec": {"memoryGB": 16}}
VA = {"version": "8.1", "highAvailability": True, "readReplicas": 1, "providerConfigs": [EC, OCC]}
create("Valkey valid spec", "ValkeyServer", VA)
create("Valkey 8 readReplicas accepted, left to the implementation", "ValkeyServer",
       w(VA, readReplicas=8))
create("Valkey retentionDays 0 accepted, backups off", "ValkeyServer",
       w(VA, backup={"retentionDays": 0}))
create("Valkey highAvailability with 0 readReplicas rejected", "ValkeyServer",
       w(VA, readReplicas=0), False, "highAvailability needs")
create("Valkey highAvailability without readReplicas rejected", "ValkeyServer",
       w(VA, readReplicas=None), False, "highAvailability needs")
create("Valkey replicas, the former name, rejected", "ValkeyServer", w(VA, replicas=1), False,
       "unknown field")
create("Valkey a full version rejected", "ValkeyServer", w(VA, version="8.1.1"), False,
       "should match")
update("Valkey raising the minor accepted", "ValkeyServer", w(VA, version="8.0"), VA)
update("Valkey lowering version rejected", "ValkeyServer", VA, w(VA, version="8.0"), False,
       "cannot decrease")
update("Valkey lowering major rejected", "ValkeyServer", VA, w(VA, version="7.2"), False,
       "cannot decrease")
update("Valkey 9.0 to 10.0 accepted", "ValkeyServer", w(VA, version="9.0"), w(VA, version="10.0"))
create("Valkey observe with an entry accepted", "ValkeyServer",
       {"observe": True, "providerConfigs": [{**EC, "spec": {"id": "orders-cache"}}]})
create("Valkey observe with a common field rejected", "ValkeyServer",
       {"observe": True, "version": "8.1", "providerConfigs": [{**EC, "spec": {"id": "x"}}]}, False,
       "takes no other common field")
create("ValkeyClass valid class", "ValkeyClass",
       {"template": {"version": "8.1", "providerConfig": EC}, "enforced": {"deletionProtection": True}})
create("ValkeyParameters accepted", "ValkeyParameters",
       {"parameters": {"maxmemory-policy": "allkeys-lru"}})
for kind, spec in {"ValkeyElastiCacheConfig": {"instanceType": "cache.t4g.small",
                                               "parameterGroupName": "orders-valkey8"},
                   "ValkeyMemorystoreConfig": {"instanceType": "HIGHMEM_MEDIUM"},
                   "ValkeyOCICacheConfig": {"memoryGB": 16}}.items():
    create(f"{kind} valid spec", kind, spec)
    create(f"{kind} unknown field rejected", kind, {**spec, "unknownSetting": 1}, False,
           "unknown field")
update("ValkeyElastiCacheConfig changing fromBackup rejected", "ValkeyElastiCacheConfig",
       {"fromBackup": "a"}, {"fromBackup": "b"}, False, "set at creation only")

print("\n# KafkaCluster")
create("gcp 30 GB for 3 vCPUs accepted, left to the cloud", "KafkaCluster",
       {"gcp": {"vcpus": 3, "memoryGB": 30}})

print("\n# KafkaUser")
KU = {"clusterRef": {"name": "c1"}, "acls": [{"topic": "orders", "operation": "Read"}]}
create("clusterRef to another namespace accepted", "KafkaUser",
       w(KU, clusterRef={"name": "c1", "namespace": "platform"}))
create("clusterRef without name rejected", "KafkaUser", w(KU, clusterRef={"namespace": "p"}),
       False, "Required value")
create("valid spec", "KafkaUser", KU)
create("missing clusterRef rejected", "KafkaUser", {"acls": []}, False, "Required value")
create("unknown operation rejected", "KafkaUser",
       {"clusterRef": {"name": "c1"}, "acls": [{"topic": "orders", "operation": "Consume"}]},
       False, "Unsupported value")
update("changing clusterRef rejected", "KafkaUser", KU, w(KU, clusterRef={"name": "c2"}), False,
       "clusterRef is immutable")
update("changing clusterRef namespace rejected", "KafkaUser", KU,
       w(KU, clusterRef={"name": "c1", "namespace": "platform"}), False, "clusterRef is immutable")
create("a prefixed ACL and consumer group accepted", "KafkaUser",
       w(KU, acls=[{"topic": "orders-", "patternType": "Prefixed", "operation": "Read"}],
         consumerGroups=[{"name": "orders-", "patternType": "Prefixed"}]))
create("an unknown patternType rejected", "KafkaUser",
       w(KU, acls=[{"topic": "orders", "patternType": "Match", "operation": "Read"}]), False,
       "Unsupported value")
create("a consumer group as a plain name, today's form, rejected", "KafkaUser",
       w(KU, consumerGroups=["orders"]), False, "must be of type object")

print("\n# RabbitMQUser and RabbitMQVirtualHost")
RU = {"brokerRef": {"name": "b1"},
      "permissions": [{"virtualHost": "orders", "configure": "^orders\\.", "write": "^orders\\.",
                       "read": "^orders\\."}]}
create("RabbitMQUser valid spec", "RabbitMQUser", RU)
create("RabbitMQUser without brokerRef rejected", "RabbitMQUser", {"permissions": []}, False,
       "Required value")
create("RabbitMQUser permission without virtualHost rejected", "RabbitMQUser",
       {"brokerRef": {"name": "b1"}, "permissions": [{"read": ".*"}]}, False, "Required value")
create("RabbitMQUser two entries for one virtual host rejected", "RabbitMQUser",
       {"brokerRef": {"name": "b1"}, "permissions": [{"virtualHost": "orders"},
                                                     {"virtualHost": "orders"}]}, False,
       "Duplicate value")
create("RabbitMQUser with secretName accepted", "RabbitMQUser",
       w(RU, secretName="orders-mq-login"))
update("RabbitMQUser changing brokerRef rejected", "RabbitMQUser", RU,
       w(RU, brokerRef={"name": "b2"}), False, "brokerRef is immutable")
update("RabbitMQUser renaming rejected", "RabbitMQUser", w(RU, name="a"), w(RU, name="b"), False,
       "name is immutable")
update("RabbitMQUser changing permissions accepted", "RabbitMQUser", RU,
       w(RU, permissions=[{"virtualHost": "orders", "read": ".*"}]))
VH = {"brokerRef": {"name": "b1"}}
create("RabbitMQVirtualHost valid spec", "RabbitMQVirtualHost", VH)
create("RabbitMQVirtualHost without brokerRef rejected", "RabbitMQVirtualHost", {}, False,
       "Required value")
update("RabbitMQVirtualHost renaming rejected", "RabbitMQVirtualHost", w(VH, name="a"),
       w(VH, name="b"), False, "name is immutable")
update("RabbitMQVirtualHost changing brokerRef rejected", "RabbitMQVirtualHost", VH,
       w(VH, brokerRef={"name": "b2"}), False, "brokerRef is immutable")

print("\n# KafkaTopic")
KT = {"clusterRef": {"name": "c1"}, "partitions": 3}
update("more partitions accepted", "KafkaTopic", KT, w(KT, partitions=6))
create("topic name with underscore and capitals accepted", "KafkaTopic", w(KT, name="Orders_v2"))
create("topic name with a slash rejected", "KafkaTopic", w(KT, name="orders/v2"), False, "should match")
update("renaming a topic rejected", "KafkaTopic", w(KT, name="a"), w(KT, name="b"), False,
       "name is immutable")
update("fewer partitions rejected", "KafkaTopic", KT, w(KT, partitions=2), False,
       "partitions cannot decrease")

print("\n# RabbitMQBroker")
create("valid spec", "RabbitMQBroker", {"version": "4.2", "maintenance": {"window": {
    "day": "Sun", "hourUTC": 3}}})
create("window without hourUTC rejected", "RabbitMQBroker",
       {"maintenance": {"window": {"day": "Sun"}}}, False, "Required value")
create("observe without id rejected", "RabbitMQBroker", {"observe": True}, False, "go together")
create("RabbitMQ public broker accepted", "RabbitMQBroker", {"aws": {"publiclyAccessible": True}})
update("RabbitMQ changing publiclyAccessible rejected", "RabbitMQBroker",
       {"aws": {"publiclyAccessible": True}}, {"aws": {"publiclyAccessible": False}}, False,
       "aws.publiclyAccessible is immutable")

print("\n# Bucket")
B = {"versioning": True, "retention": {"days": 30, "locked": False},
     "lifecycleRules": [{"prefix": "logs/", "transitions": [{"afterDays": 90, "tier": "Archive"}]}]}
create("valid spec", "Bucket", B)
create("unknown tier rejected", "Bucket",
       w(B, lifecycleRules=[{"transitions": [{"afterDays": 90, "tier": "Glacier"}]}]), False, "Unsupported value")
update("locking retention accepted", "Bucket", B, w(B, retention={"days": 30, "locked": True}))
L = w(B, retention={"days": 30, "locked": True})
update("unlocking retention rejected", "Bucket", L, B, False, "cannot be unlocked")
update("removing locked retention rejected", "Bucket", L, w(L, retention=None), False, "cannot be unlocked or removed")

print("\n# ArtifactRepository")
CR = {"name": "orders-api", "path": "shop",
      "lifecycleRules": [{"tagPrefixes": ["develop"], "keepCount": 10},
                         {"untagged": True, "expireAfterDays": 7}, {"keepCount": 50}]}
create("valid spec", "ArtifactRepository", CR)
create("two selectors rejected", "ArtifactRepository",
       w(CR, lifecycleRules=[{"tagPrefixes": ["a"], "untagged": True, "keepCount": 1}]), False,
       "at most one selector")
create("two bounds rejected", "ArtifactRepository",
       w(CR, lifecycleRules=[{"keepCount": 1, "expireAfterDays": 1}]), False, "exactly one")
create("no bound rejected", "ArtifactRepository", w(CR, lifecycleRules=[{"untagged": True}]),
       False, "exactly one")
update("changing name rejected", "ArtifactRepository", CR, w(CR, name="x"), False,
       "name is immutable")
update("changing path rejected", "ArtifactRepository", CR, w(CR, path="x"), False,
       "path is immutable")
update("adding path later rejected", "ArtifactRepository", w(CR, path=None), CR, False, "path is immutable")
update("removing name rejected", "ArtifactRepository", CR, w(CR, name=None), False, "name is immutable")


print("\n# Portability: one object applies on every cloud")
SIZES = {"KafkaCluster": {"aws": {"instanceType": "kafka.m7g.large"},
                          "gcp": {"vcpus": 3, "memoryGB": 12},
                          "oci": {"instanceType": "VM.Standard.E5.Flex", "ocpus": 2}},
         "RabbitMQBroker": {"aws": {"instanceType": "mq.m7g.medium"}}}
for kind, spec in SIZES.items():
    create(f"{kind}: a size for every cloud in one object accepted", kind, spec)
    create(f"{kind}: instanceType outside the cloud blocks rejected", kind,
           {"instanceType": "db.t4g.small"}, False, "unknown field")

print("\n# Shared rules on every resource kind")
BLOCKS = {"KafkaCluster": ["aws", "gcp", "oci"],
          "RabbitMQBroker": ["aws"], "Bucket": ["aws", "gcp", "oci", "azure"],
          "ArtifactRepository": ["aws", "gcp", "oci", "azure"]}
for kind, blocks in BLOCKS.items():
    if len(blocks) > 1:
        create(f"{kind}: two cloud blocks accepted", kind, {blocks[0]: {}, blocks[1]: {}})
        create(f"{kind}: ids in two cloud blocks rejected", kind,
               {"observe": True, blocks[0]: {"id": "a"}, blocks[1]: {"id": "b"}}, False,
               "at most one cloud block may hold an id")
    for c in blocks:
        create(f"{kind}: observe without {c}.id rejected", kind, {"observe": True, c: {}}, False,
               "go together")
        create(f"{kind}: {c}.id without observe rejected", kind, {c: {"id": "x"}}, False,
               "go together")
        ob = {"observe": True, c: {"id": "x"}}
        create(f"{kind}: observing by {c}.id accepted", kind, ob)
        update(f"{kind}: changing {c}.id rejected", kind, ob, {"observe": True, c: {"id": "y"}},
               False, "id is immutable")
        update(f"{kind}: removing observe with {c}.id rejected", kind, ob, {c: {"id": "x"}}, False,
               "observe is immutable")

print("\n# More KafkaTopic rules")
update("KafkaTopic changing clusterRef rejected", "KafkaTopic", {"clusterRef": {"name": "c1"}},
       {"clusterRef": {"name": "c2"}}, False, "clusterRef is immutable")


print("\n# Users and databases inside an instance")
PU = {"clusterRef": {"name": "orders-db"}}
create("PostgreSQLUser valid spec", "PostgreSQLUser", w(PU, name="orders_app", login=True))
create("PostgreSQLUser without clusterRef rejected", "PostgreSQLUser", {"login": True}, False,
       "Required value")
update("PostgreSQLUser changing clusterRef rejected", "PostgreSQLUser", PU,
       {"clusterRef": {"name": "other-db"}}, False, "clusterRef is immutable")
update("PostgreSQLUser changing name rejected", "PostgreSQLUser", w(PU, name="a"), w(PU, name="b"),
       False, "name is immutable")
PD = {"clusterRef": {"name": "orders-db"}, "owner": "orders_app"}
create("PostgreSQLDatabase valid spec", "PostgreSQLDatabase",
       w(PD, extensions=["postgis"], extensionConfig={"postgis": {"schema": "gis"}}))
create("PostgreSQLDatabase without owner rejected", "PostgreSQLDatabase",
       {"clusterRef": {"name": "orders-db"}}, False, "Required value")
create("PostgreSQLDatabase unknown extension rejected", "PostgreSQLDatabase",
       w(PD, extensions=["pg_cron"]), False, "Unsupported value")
update("PostgreSQLDatabase adding name later rejected", "PostgreSQLDatabase", PD,
       w(PD, name="orders"), False, "name is immutable")
MU = {"serverRef": {"name": "orders-db"},
      "databases": [{"databaseRef": {"name": "orders"}, "privileges": ["SELECT", "INSERT"]}]}
create("MariaDBUser valid spec", "MariaDBUser", MU)
create("MariaDBUser on two databases accepted", "MariaDBUser",
       w(MU, databases=MU["databases"] + [{"databaseRef": {"name": "invoices"},
                                           "privileges": ["SELECT"]}]))
create("MariaDBUser one database twice rejected", "MariaDBUser",
       w(MU, databases=MU["databases"] * 2), False, "each database and table appears once")
create("MariaDBUser one database on two tables accepted", "MariaDBUser",
       w(MU, databases=[{"databaseRef": {"name": "orders"}, "table": "a", "privileges": ["SELECT"]},
                        {"databaseRef": {"name": "orders"}, "table": "b", "privileges": ["SELECT"]}]))
create("MariaDBUser without databases rejected", "MariaDBUser", w(MU, databases=None), False,
       "Required value")
create("MariaDBUser entry without privileges rejected", "MariaDBUser",
       w(MU, databases=[{"databaseRef": {"name": "orders"}}]), False, "Required value")
create("MariaDBUser lowercase privilege rejected", "MariaDBUser",
       w(MU, databases=[{"databaseRef": {"name": "orders"}, "privileges": ["select"]}]),
       False, "should match")
update("MariaDBUser adding a database accepted", "MariaDBUser", MU,
       w(MU, databases=MU["databases"] + [{"databaseRef": {"name": "invoices"},
                                           "privileges": ["SELECT"]}]))
MD = {"serverRef": {"name": "orders-db"}}
create("MariaDBDatabase valid spec", "MariaDBDatabase", w(MD, defaultCharacterSet="utf8mb4"))
update("MariaDBDatabase changing serverRef rejected", "MariaDBDatabase", MD,
       {"serverRef": {"name": "orders-db", "namespace": "shop-platform"}}, False,
       "serverRef is immutable")


print("\n# Connection secret names")
create("PostgreSQLUser with secretName accepted", "PostgreSQLUser",
       {"clusterRef": {"name": "orders-db"}, "secretName": "orders-app-db"})
create("KafkaUser with secretName accepted", "KafkaUser",
       {"clusterRef": {"name": "c1"}, "secretName": "orders-kafka-login"})

print(f"\n{sum(results)} of {len(results)} passed")
