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
        crd = {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
               "metadata": {"name": x["metadata"]["name"]},
               "spec": {"group": s["group"], "scope": s["scope"],
                        "names": {"kind": s["names"]["kind"], "plural": s["names"]["plural"]},
                        "versions": [{"name": v["name"], "served": True, "storage": True,
                                      "schema": v["schema"]}]}}
        code, out = kubectl("apply", "-f", "-", stdin=json.dumps(crd))
        print(("INSTALLED " if code == 0 else "INSTALL FAILED ") + f.name + ("" if code == 0 else "\n" + out))
        groups[s["names"]["kind"]] = s["group"] + "/" + v["name"]
    kubectl("wait", "--for=condition=Established", "crd", "--all", "--timeout=60s")
    return groups


GV = install()
results = []
n = 0


def obj(kind, spec):
    global n
    n += 1
    return {"apiVersion": GV[kind], "kind": kind,
            "metadata": {"name": f"t{n}", "namespace": NS}, "spec": spec}


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


PG = {"version": "17", "instanceType": "db.t4g.small",
      "storage": {"sizeGB": 20, "autoGrow": True, "maxSizeGB": 100},
      "highAvailability": False, "deletionProtection": True,
      "maintenance": {"window": {"day": "Sun", "hourUTC": 3}, "autoMinorVersionUpgrade": True,
                      "allowMajorVersionUpgrade": False},
      "backup": {"retentionDays": 7, "startHourUTC": 2},
      "parameters": {"max_connections": "200"}, "gcp": {"edition": "ENTERPRISE"}}


def w(base, **kw):
    s = copy.deepcopy(base)
    for k, v in kw.items():
        if v is None:
            s.pop(k, None)
        else:
            s[k] = v
    return s


print("\n# PostgreSQLInstance")
create("valid spec", "PostgreSQLInstance", PG)
create("two cloud blocks accepted, each applies on its own cloud", "PostgreSQLInstance",
       w(PG, oci={"ocpus": 2}))
create("ids in two cloud blocks rejected", "PostgreSQLInstance",
       {"observe": True, "aws": {"id": "a"}, "gcp": {"id": "b"}}, False, "at most one cloud block")
create("observe without id rejected", "PostgreSQLInstance", {"observe": True, "aws": {}}, False,
       "go together")
create("observe with aws.id accepted", "PostgreSQLInstance",
       {"observe": True, "aws": {"id": "arn:aws:rds:x"}})
create("maxSizeGB below sizeGB rejected", "PostgreSQLInstance",
       w(PG, storage={"sizeGB": 200, "maxSizeGB": 100}), False, "maxSizeGB must be")
create("retentionDays 3 with pointInTimeDays accepted", "PostgreSQLInstance",
       w(PG, backup={"retentionDays": 3, "pointInTimeDays": 3}))
create("retentionDays above every cloud's range accepted, left to the cloud",
       "PostgreSQLInstance", w(PG, backup={"retentionDays": 400, "pointInTimeDays": 40}))
create("retentionDays 0 accepted, backups off", "PostgreSQLInstance",
       w(PG, backup={"retentionDays": 0}))
create("retentionDays -1 rejected", "PostgreSQLInstance", w(PG, backup={"retentionDays": -1}),
       False, "greater than or equal to 0")
create("pointInTimeDays with retentionDays 0 rejected", "PostgreSQLInstance",
       w(PG, backup={"retentionDays": 0, "pointInTimeDays": 3}), False, "needs backups")
create("MariaDB retentionDays 0 accepted, backups off", "MariaDBInstance",
       {"backup": {"retentionDays": 0}})
create("minor version rejected", "PostgreSQLInstance", w(PG, version="17.2"), False, "takes a minor exactly")
create("unknown storageTier accepted, left to the cloud", "PostgreSQLInstance",
       w(PG, gcp=None, azure={"storageTier": "P5"}))
create("MariaDB retentionDays 3 accepted", "MariaDBInstance",
       {"backup": {"retentionDays": 3}})
create("restore without fromInstance rejected", "PostgreSQLInstance",
       w(PG, restore={"pointInTime": "2026-11-01T10:00:00Z"}), False, "pointInTime needs fromInstance")
update("major version change rejected", "PostgreSQLInstance", PG, w(PG, version="18"), False,
       "a major version change needs")
up = w(PG, maintenance={"allowMajorVersionUpgrade": True})
update("major version change with allowMajorVersionUpgrade accepted", "PostgreSQLInstance", up,
       w(up, version="18"))
update("other field change accepted", "PostgreSQLInstance", PG, w(PG, instanceType="db.m7g.large"))
rs = w(PG, restore={"fromInstance": "old-db"})
update("adding restore rejected", "PostgreSQLInstance", PG, rs, False, "restore is set")
update("removing restore rejected", "PostgreSQLInstance", rs, PG, False, "restore is set")
update("changing restore rejected", "PostgreSQLInstance", rs,
       w(rs, restore={"fromInstance": "other"}), False, "restore is set")
update("unchanged restore accepted", "PostgreSQLInstance", rs, w(rs, instanceType="db.m7g.large"))
ob = {"observe": True, "aws": {"id": "arn:aws:rds:x"}}
update("changing observe rejected", "PostgreSQLInstance", ob, w(ob, observe=False), False,
       "observe is immutable")
update("changing aws.id rejected", "PostgreSQLInstance", ob, w(ob, aws={"id": "arn:aws:rds:y"}),
       False, "id is immutable")
create("id without observe rejected", "PostgreSQLInstance", w(PG, gcp=None, aws={"id": "a"}),
       False, "go together")
update("adding observe later rejected", "PostgreSQLInstance", w(PG, gcp=None),
       {"observe": True, "aws": {"id": "a"}}, False, "observe is immutable")
update("removing aws.id from an observed object rejected", "PostgreSQLInstance", ob,
       w(ob, aws={}), False, "go together")
update("removing observe rejected", "PostgreSQLInstance", ob, w(ob, observe=None), False,
       "observe is immutable")

create("aws.parameterGroupName accepted", "PostgreSQLInstance",
       w(PG, gcp=None, parameters=None, aws={"parameterGroupName": "pg17"}))
create("aws.parameterGroupName with parameters rejected", "PostgreSQLInstance",
       w(PG, gcp=None, aws={"parameterGroupName": "pg17"}), False, "cannot be used together")
update("changing aws.parameterGroupName accepted", "PostgreSQLInstance",
       w(PG, gcp=None, parameters=None, aws={"parameterGroupName": "a"}),
       w(PG, gcp=None, parameters=None, aws={"parameterGroupName": "b"}))
create("restore.fromBackup accepted", "PostgreSQLInstance", w(PG, restore={"fromBackup": "rds:x"}))
create("restore with fromInstance and fromBackup rejected", "PostgreSQLInstance",
       w(PG, restore={"fromInstance": "a", "fromBackup": "b"}), False, "exactly one")
create("restore with neither rejected", "PostgreSQLInstance",
       w(PG, restore={"pointInTime": "2026-11-01T10:00:00Z"}), False, "exactly one")
create("pointInTime with fromBackup rejected", "PostgreSQLInstance",
       w(PG, restore={"fromBackup": "b", "pointInTime": "2026-11-01T10:00:00Z"}), False,
       "pointInTime needs")
fb = w(PG, restore={"fromBackup": "rds:x"})
update("changing restore.fromBackup rejected", "PostgreSQLInstance", fb,
       w(fb, restore={"fromBackup": "rds:y"}), False, "restore is set")
update("adding restore.fromBackup later rejected", "PostgreSQLInstance", PG, fb, False,
       "restore is set")
create("aws.snapshotIdentifier rejected", "PostgreSQLInstance",
       w(PG, gcp=None, aws={"snapshotIdentifier": "s1"}), False, "unknown field")
create("MariaDB restore.fromBackup rejected", "MariaDBInstance",
       {"restore": {"fromInstance": "a", "fromBackup": "b"}}, False, "exactly one of fromInstance and fromBackup")

update("raising storage.sizeGB accepted", "PostgreSQLInstance", PG,
       w(PG, storage={"sizeGB": 50, "maxSizeGB": 100}))
update("lowering storage.sizeGB rejected", "PostgreSQLInstance", PG,
       w(PG, storage={"sizeGB": 10, "maxSizeGB": 100}), False, "cannot decrease")

create("parameterApplyMethod NextRestart accepted", "PostgreSQLInstance",
       w(PG, parameterApplyMethod="NextRestart"))
create("unknown parameterApplyMethod rejected", "PostgreSQLInstance",
       w(PG, parameterApplyMethod="Later"), False, "Unsupported value")

pg = w(PG, gcp=None, parameters=None, aws={"parameterGroupName": "pg17"},
       maintenance={"allowMajorVersionUpgrade": True})
update("version change keeping aws.parameterGroupName rejected", "PostgreSQLInstance", pg,
       w(pg, version="18"), False, "needs a new aws.parameterGroupName")
update("version change with a new aws.parameterGroupName accepted", "PostgreSQLInstance", pg,
       w(pg, version="18", aws={"parameterGroupName": "pg18"}))
update("version change removing aws.parameterGroupName accepted", "PostgreSQLInstance", pg,
       w(pg, version="18", aws=None))

pin = w(PG, version="17.7", maintenance={"autoMinorVersionUpgrade": False})
create("full version with minor upgrades off accepted", "PostgreSQLInstance", pin)
create("major-only version with minor upgrades off rejected", "PostgreSQLInstance",
       w(pin, version="17"), False, "takes a minor exactly when")
create("full version with minor upgrades on rejected", "PostgreSQLInstance",
       w(PG, version="17.7"), False, "takes a minor exactly when")
update("minor change without allowMajorVersionUpgrade accepted", "PostgreSQLInstance", pin,
       w(pin, version="17.8"))
update("major change of a pinned version rejected", "PostgreSQLInstance", pin,
       w(pin, version="18.1"), False, "a major version change needs")
pgm = w(pin, gcp=None, parameters=None, aws={"parameterGroupName": "pg17"})
update("minor change keeping aws.parameterGroupName accepted", "PostgreSQLInstance", pgm,
       w(pgm, version="17.8"))

print("\n# MariaDBInstance")
create("two-number version accepted", "MariaDBInstance", {"version": "11.4", "aws": {}})
create("one-number version rejected", "MariaDBInstance", {"version": "11"}, False, "should match")
MA = {"version": "11.4", "aws": {"parameterGroupName": "orders-mariadb114"}}
create("MariaDB restore.fromBackup accepted", "MariaDBInstance",
       {"restore": {"fromBackup": "orders-final"}})
create("MariaDB aws.parameterGroupName with parameters rejected", "MariaDBInstance",
       w(MA, parameters={"max_connections": "200"}), False, "cannot be used together")
MAU = w(MA, maintenance={"allowMajorVersionUpgrade": True})
update("MariaDB major change keeping the parameter group rejected", "MariaDBInstance", MAU,
       w(MAU, version="11.8"), False, "needs a new aws.parameterGroupName")
update("MariaDB major change with a new parameter group accepted", "MariaDBInstance", MAU,
       w(MAU, version="11.8", aws={"parameterGroupName": "orders-mariadb118"}))
MP = {"version": "11.4.10", "maintenance": {"autoMinorVersionUpgrade": False}, "aws": {}}
create("MariaDB full version with minor upgrades off accepted", "MariaDBInstance", MP)
create("MariaDB full version with minor upgrades on rejected", "MariaDBInstance",
       w(MP, maintenance=None), False, "takes a minor exactly")
create("MariaDB major only with minor upgrades off rejected", "MariaDBInstance",
       w(MP, version="11.4"), False, "takes a minor exactly")
update("MariaDB minor change accepted", "MariaDBInstance", MP, w(MP, version="11.4.11"))
update("MariaDB major change without allowMajorVersionUpgrade rejected", "MariaDBInstance", MP,
       w(MP, version="11.8.3"), False, "allowMajorVersionUpgrade")
update("MariaDB lowering storage.sizeGB rejected", "MariaDBInstance",
       w(MA, storage={"sizeGB": 50}), w(MA, storage={"sizeGB": 40}), False, "cannot decrease")

print("\n# ValkeyInstance")
VA = {"version": "8.1", "aws": {"parameterGroupName": "orders-valkey8"}}
create("Valkey aws.parameterGroupName accepted", "ValkeyInstance", VA)
create("Valkey 8 replicas accepted, left to the cloud", "ValkeyInstance",
       {"replicas": 8, "aws": {}})
create("Valkey retentionDays 0 accepted, backups off", "ValkeyInstance",
       {"backup": {"retentionDays": 0}, "aws": {}})
create("Valkey aws.parameterGroupName with parameters rejected", "ValkeyInstance",
       w(VA, parameters={"maxmemory-policy": "allkeys-lru"}), False, "cannot be used together")
update("Valkey major change keeping the parameter group rejected", "ValkeyInstance", VA,
       w(VA, version="9.0"), False, "needs a new aws.parameterGroupName")
update("Valkey minor change keeping the parameter group accepted", "ValkeyInstance",
       w(VA, version="8.0"), VA)
update("Valkey lowering version rejected", "ValkeyInstance", VA, w(VA, version="8.0"), False,
       "cannot decrease")
update("Valkey lowering major rejected", "ValkeyInstance", {"version": "8.1"},
       {"version": "7.2"}, False, "cannot decrease")
update("Valkey 9.0 to 10.0 accepted", "ValkeyInstance", {"version": "9.0"}, {"version": "10.0"})
create("valid spec", "ValkeyInstance", {"version": "8.1", "highAvailability": True, "replicas": 1,
                                        "oci": {"memoryGB": 16}})
create("highAvailability with 0 replicas rejected", "ValkeyInstance",
       {"highAvailability": True, "replicas": 0}, False, "highAvailability needs")
create("highAvailability without replicas rejected", "ValkeyInstance",
       {"highAvailability": True}, False, "highAvailability needs")
create("azure block rejected", "ValkeyInstance", {"azure": {}}, False, "unknown field")

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


print("\n# Shared rules on every resource kind")
BLOCKS = {"PostgreSQLInstance": ["aws", "gcp", "oci", "azure"], "MariaDBInstance": ["aws"],
          "ValkeyInstance": ["aws", "gcp", "oci"], "KafkaCluster": ["aws", "gcp", "oci"],
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

print("\n# More MariaDBInstance and KafkaTopic rules")
create("MariaDB maxSizeGB below sizeGB rejected", "MariaDBInstance",
       {"storage": {"sizeGB": 100, "maxSizeGB": 50}}, False, "maxSizeGB must be at least sizeGB")
create("MariaDB pointInTime with fromBackup rejected", "MariaDBInstance",
       {"restore": {"fromBackup": "b", "pointInTime": "2026-11-01T10:00:00Z"}}, False,
       "pointInTime needs fromInstance")
update("MariaDB adding restore later rejected", "MariaDBInstance", {"version": "11.4"},
       {"version": "11.4", "restore": {"fromInstance": "a"}}, False, "restore is set at creation only")
update("KafkaTopic changing clusterRef rejected", "KafkaTopic", {"clusterRef": {"name": "c1"}},
       {"clusterRef": {"name": "c2"}}, False, "clusterRef is immutable")


print("\n# Users and databases inside an instance")
PU = {"instanceRef": {"name": "orders-db"}}
create("PostgreSQLUser valid spec", "PostgreSQLUser", w(PU, name="orders_app", login=True))
create("PostgreSQLUser without instanceRef rejected", "PostgreSQLUser", {"login": True}, False,
       "Required value")
update("PostgreSQLUser changing instanceRef rejected", "PostgreSQLUser", PU,
       {"instanceRef": {"name": "other-db"}}, False, "instanceRef is immutable")
update("PostgreSQLUser changing name rejected", "PostgreSQLUser", w(PU, name="a"), w(PU, name="b"),
       False, "name is immutable")
PD = {"instanceRef": {"name": "orders-db"}, "owner": "orders_app"}
create("PostgreSQLDatabase valid spec", "PostgreSQLDatabase",
       w(PD, extensions=["postgis"], extensionConfig={"postgis": {"schema": "gis"}}))
create("PostgreSQLDatabase without owner rejected", "PostgreSQLDatabase",
       {"instanceRef": {"name": "orders-db"}}, False, "Required value")
create("PostgreSQLDatabase unknown extension rejected", "PostgreSQLDatabase",
       w(PD, extensions=["pg_cron"]), False, "Unsupported value")
update("PostgreSQLDatabase adding name later rejected", "PostgreSQLDatabase", PD,
       w(PD, name="orders"), False, "name is immutable")
MU = {"instanceRef": {"name": "orders-db"}, "databaseRef": {"name": "orders"},
      "privileges": ["SELECT", "INSERT"]}
create("MariaDBUser valid spec", "MariaDBUser", MU)
create("MariaDBUser without privileges rejected", "MariaDBUser", w(MU, privileges=None), False,
       "Required value")
create("MariaDBUser lowercase privilege rejected", "MariaDBUser", w(MU, privileges=["select"]),
       False, "should match")
update("MariaDBUser changing databaseRef rejected", "MariaDBUser", MU,
       w(MU, databaseRef={"name": "orders", "namespace": "shop-billing"}), False,
       "databaseRef is immutable")
MD = {"instanceRef": {"name": "orders-db"}}
create("MariaDBDatabase valid spec", "MariaDBDatabase", w(MD, defaultCharacterSet="utf8mb4"))
update("MariaDBDatabase changing instanceRef rejected", "MariaDBDatabase", MD,
       {"instanceRef": {"name": "orders-db", "namespace": "shop-platform"}}, False,
       "instanceRef is immutable")


print("\n# Connection secret names")
create("PostgreSQLUser with secretName accepted", "PostgreSQLUser",
       {"instanceRef": {"name": "orders-db"}, "secretName": "orders-app-db"})
create("KafkaUser with secretName accepted", "KafkaUser",
       {"clusterRef": {"name": "c1"}, "secretName": "orders-kafka-login"})
create("PostgreSQLInstance with secretName accepted", "PostgreSQLInstance",
       {"secretName": "orders-db-admin", "aws": {}})

print(f"\n{sum(results)} of {len(results)} passed")
