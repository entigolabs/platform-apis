"""Generate the draft XRD schemas for the data service API design.

Usage: python3 generate.py <schemas directory>

Writes one <kind>.yaml per kind, overwriting it. Edit the schemas here, not in the YAML, then run
test_rules.py.
"""
import sys
import textwrap
from pathlib import Path

import yaml

OUT = Path(sys.argv[1])
CLOUD_NAMES = {"aws": "AWS", "gcp": "GCP", "oci": "OCI", "azure": "Azure"}


class Literal(str):
    pass


class Dumper(yaml.SafeDumper):
    def choose_scalar_style(self):
        style = super().choose_scalar_style()
        return '"' if style == "'" else style

    def increase_indent(self, flow=False, indentless=False):
        return super().increase_indent(flow, indentless)


Dumper.add_representer(
    Literal, lambda d, v: d.represent_scalar("tag:yaml.org,2002:str", v, style="|"))


def desc(text, ext=None, default=None, single_cloud=False):
    """A description: the text, then the support level and default lines."""
    lines = textwrap.wrap(text, 80) if text else []
    if ext and not single_cloud:
        lines.append(ext)
    if default:
        lines.append("Default: " + default + ".")
    return Literal("\n".join(lines))


def field(type_, text=None, ext=None, default=None, sc=False, **kw):
    f = {"type": type_}
    if text or ext or default:
        f["description"] = desc(text, ext, default, sc)
    f.update(kw)
    return f


def obj(props, text=None, ext=None, default=None, sc=False, required=None, rules=None):
    f = {"type": "object"}
    if text or ext or default:
        f["description"] = desc(text, ext, default, sc)
    f["properties"] = props
    if required:
        f["required"] = required
    if rules:
        f["x-kubernetes-validations"] = [{"rule": cel(r), "message": m} for r, m in rules]
    return f


def cel(rule):
    """Break a long CEL rule before each top-level || into a literal block."""
    if len(rule) <= 70:
        return rule
    parts, depth, cur = [], 0, ""
    i = 0
    while i < len(rule):
        c = rule[i]
        depth += c in "([" and 1 or c in ")]" and -1 or 0
        if depth == 0 and rule.startswith(" || ", i):
            parts.append(cur)
            cur = "|| "
            i += 4
            continue
        cur += c
        i += 1
    parts.append(cur)
    return Literal("\n".join(parts))


def string_map(text, ext=None, sc=False):
    return {"type": "object", "description": desc(text, ext, None, sc),
            "additionalProperties": {"type": "string"}}


def array(items, text=None, ext=None, sc=False, default=None, **kw):
    f = {"type": "array"}
    if text or ext or default:
        f["description"] = desc(text, ext, default, sc)
    f["items"] = items
    f.update(kw)
    return f


IMMUTABLE = [("self == oldSelf", "is immutable")]


def cloud_block(cloud, kind_noun, settings=None):
    props = {"id": field("string", f"The {CLOUD_NAMES[cloud]} ID of an existing {kind_noun} "
                         "to observe (rule 9). Only with observe.",
                         **{"x-kubernetes-validations": [{"rule": "self == oldSelf",
                                                          "message": "id is immutable"}]})}
    props.update(settings or {})
    return obj(props, f"Settings for {CLOUD_NAMES[cloud]} only (rule 2).")


def resource_spec(common, blocks, kind_noun, rules=None, observe_ext=None):
    """Spec of a kind that represents a cloud resource: observe, common fields, cloud blocks."""
    props = {"observe": field(
        "boolean",
        "true observes an existing resource, named by the cloud block's id, and never changes "
        "or deletes it. Every other field is then rejected (rule 9). Set at creation only.",
        observe_ext)}
    props.update(common)
    props.update({c: cloud_block(c, kind_noun, s) for c, s in blocks.items()})
    has = [f"has(self.{c})" for c in blocks]
    ids = " || ".join(f"(has(self.{c}) && has(self.{c}.id))" for c in blocks)
    all_rules = []
    if len(blocks) > 1:
        id_set = [f"(has(self.{c}) && has(self.{c}.id))" for c in blocks]
        all_rules.append((f"[{', '.join(id_set)}].filter(b, b).size() <= 1",
                          "at most one cloud block may hold an id"))
    all_rules.append((f"(has(self.observe) && self.observe) == ({ids})",
                      "observe and the cloud block's id go together"))
    all_rules.append(("(has(self.observe) && self.observe) == "
                      "(has(oldSelf.observe) && oldSelf.observe)", "observe is immutable"))
    all_rules += rules or []
    return obj(props, rules=all_rules)


def parent_ref(kind, name):
    return obj({"name": field("string", f"The {kind}."),
                "namespace": field("string", f"The {kind}'s namespace, which must be in this "
                                   "object's Zone (rule 11). Unset, this object's "
                                   "namespace.")},
               f"The {kind} this object lives in.", required=["name"],
               rules=[("self == oldSelf", f"{name} is immutable")])


def secret_name(default):
    return field("string", "The name of the connection Secret (rule 6). The platform never "
                 "overwrites a Secret that another object or person owns; set this to avoid one.",
                 default=f"fixed, {default}")


def IMMUTABLE_MSG(name):
    return [{"rule": "self == oldSelf", "message": f"{name} is immutable"}]


def endpoint(text="The address applications connect to."):
    return obj({"host": field("string"), "port": field("integer")}, text)


def xrd(kind, plural, group, spec, status, required_spec=None):
    schema = {"type": "object", "properties": {"spec": spec, "status": status},
              "required": ["spec"]}
    if required_spec:
        spec["required"] = required_spec
    return {
        "apiVersion": "apiextensions.crossplane.io/v2",
        "kind": "CompositeResourceDefinition",
        "metadata": {"name": f"{plural}.{group}"},
        "spec": {
            "group": group,
            "scope": "Namespaced",
            "names": {"kind": kind, "plural": plural},
            "versions": [{"name": "v1alpha2", "served": True, "referenceable": True,
                          "schema": {"openAPIV3Schema": schema}}],
        },
    }


def write(name, doc):
    text = "---\n" + yaml.dump(doc, Dumper=Dumper, sort_keys=False, width=100,
                               allow_unicode=False)
    (OUT / f"{name}.yaml").write_text(text)


DAY = field("string", enum=["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"])
HOUR = field("integer", minimum=0, maximum=23)
DEPLOY = "per deployment"


def window(ext=None, sc=False):
    return obj({"day": DAY, "hourUTC": HOUR},
               "The weekly hour, in UTC, in which the cloud applies updates.", ext, None, sc,
               required=["day", "hourUTC"])


# --- PostgreSQL and MariaDB: one shared instance spec --------------------------------------

def size_field(floor):
    if not floor:
        return field("integer", "Disk size in GB at creation.", default=DEPLOY, minimum=1)
    return field("integer", "Disk size in GB at creation, and its floor after: raising it grows "
                 "the disk, lowering it is rejected. The disk can be larger: it is rounded up to a "
                 "size the cloud accepts, and autoGrow can take it further. The status shows the "
                 "real size.",
                 default=DEPLOY, minimum=1, **{"x-kubernetes-validations": [
                     {"rule": "self >= oldSelf", "message": "storage.sizeGB cannot decrease"}]})


def restore_block(kind):
    return obj({
        "fromInstance": field("string", f"A {kind} in this namespace."),
        "pointInTime": field("string", "The time to restore fromInstance to. Unset, the latest "
                             "restorable time.", format="date-time"),
    }, "Create the instance from another instance's backups. Create time only.",
        required=["fromInstance"])


def from_backup_field(text):
    return field("string", text + " Create time only; not with restore.")


def from_backup_rules(clouds):
    """fromBackup lives in the cloud blocks (rule 2): set at creation only, never with restore."""
    rules = []
    for c in clouds:
        has = f"(has(self.{c}) && has(self.{c}.fromBackup))"
        had = f"(has(oldSelf.{c}) && has(oldSelf.{c}.fromBackup))"
        rules.append((f"{has} == {had} && (!{has} || "
                      f"self.{c}.fromBackup == oldSelf.{c}.fromBackup)",
                      f"{c}.fromBackup is set at creation only"))
        rules.append((f"!{has} || !has(self.restore)",
                      f"{c}.fromBackup cannot be combined with restore"))
    return rules


def db_instance(kind, plural, engine, version_text, version_pattern, blocks, sc, extra_rules=(),
                from_backup=False, major=lambda v: v, minor_ext=None, pitr=None):
    pitr = from_backup if pitr is None else pitr
    common = {
        "version": field("string", version_text, minor_ext, DEPLOY, sc, pattern=version_pattern),
        "storage": obj({
            "sizeGB": size_field(from_backup),
            "autoGrow": field("boolean", "Grow the disk as it fills.",
                              "Extended for false: AWS, GCP, Azure.", "fixed, true", sc),
            "maxSizeGB": field("integer", "The size autoGrow stops at.",
                               "Extended: AWS, GCP.", DEPLOY, sc, minimum=1),
            "iops": field("integer", "Provisioned IOPS.",
                          "Extended: AWS, GCP with Hyperdisk, OCI.", None, sc, minimum=1),
            "throughputMBps": field("integer", "Provisioned throughput in MB/s.",
                                    "Extended: AWS, GCP with Hyperdisk.", None, sc,
                                    minimum=1),
        }, rules=[("!has(self.maxSizeGB) || !has(self.sizeGB) || self.maxSizeGB >= self.sizeGB",
                   "maxSizeGB must be at least sizeGB")]),
        "highAvailability": field("boolean", "A standby in another zone, with automatic "
                                  "failover.", default=DEPLOY),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenance": obj({
            "window": window(),
            "autoMinorVersionUpgrade": field(
                "boolean", "Apply new minor versions in the maintenance window. false keeps "
                "the minor written in version.", "Extended for false: AWS.", DEPLOY, sc),
            "allowMajorVersionUpgrade": field(
                "boolean", "Allow version to change to a later major.",
                "Extended for true: AWS, GCP, Azure.", "fixed, false", sc),
        }),
        "backup": obj({
            "retentionDays": field("integer", "Days automated backups are kept, within the "
                                   "cloud's own range, which the cloud checks (rule 10). 0 "
                                   "turns automated backups and point-in-time restore off.",
                                   "Extended for 0: AWS, GCP, OCI.", DEPLOY, sc, minimum=0),
            **({"pointInTimeDays": field(
                "integer", "Days back restore.pointInTime can reach, within the cloud's own "
                "range. AWS and Azure keep one window for both, so above retentionDays is "
                "rejected there.",
                default="fixed, the lower of retentionDays and the cloud's maximum; none "
                "with retentionDays 0",
                minimum=1)} if pitr else {}),
            "startHourUTC": field("integer", "The hour, in UTC, the daily backup starts.",
                                  "Extended: AWS, GCP, OCI.", None, sc, minimum=0,
                                  maximum=23),
        }, rules=[("!has(self.pointInTimeDays) || !has(self.retentionDays) || "
                   "self.retentionDays > 0",
                   "pointInTimeDays needs backups, which retentionDays 0 disables")]
           if pitr else None),
        "parameters": string_map(f"{engine} settings, by name. Keys outside the cloud's "
                                 "allowlist are rejected."),
        **({"parameterApplyMethod": field(
            "string", "When changed parameters take effect. Immediate restarts the instance "
            "where a setting needs it; RDS rejects such settings with Immediate.",
            "Extended for NextRestart: AWS.", "fixed, Immediate", sc,
            enum=["Immediate", "NextRestart"])} if from_backup else {}),
        "restore": restore_block(kind),
        "secretName": secret_name("<instance>-dbadmin"),
    }
    rules = [
        ("has(self.restore) == has(oldSelf.restore) && (!has(self.restore) || "
         "self.restore == oldSelf.restore)", "restore is set at creation only"),
        ("!has(self.version) || !has(oldSelf.version) || "
         f"{major('self.version')} == {major('oldSelf.version')} || "
         "(has(self.maintenance) && has(self.maintenance.allowMajorVersionUpgrade) && "
         "self.maintenance.allowMajorVersionUpgrade)",
         "a major version change needs maintenance.allowMajorVersionUpgrade"),
    ]
    spec = resource_spec(common, blocks, "database instance", rules + list(extra_rules))
    status = obj({
        "version": field("string"),
        "storage": obj({"sizeGB": field("integer", "The actual disk size.") if from_backup
                        else field("integer"), "autoGrow": field("boolean"),
                        "maxSizeGB": field("integer"), "iops": field("integer"),
                        "throughputMBps": field("integer")}),
        "highAvailability": field("boolean"),
        "maintenance": obj({"window": obj({"day": field("string"), "hourUTC": field("integer")}),
                            "autoMinorVersionUpgrade": field("boolean")}),
        "backup": obj({"retentionDays": field("integer"),
                       **({"pointInTimeDays": field("integer")} if pitr else {}),
                       "startHourUTC": field("integer")}),
        "endpoint": endpoint(),
        "connectionSecret": field("string", f"The admin Secret, <instance>-dbadmin (rule 6)."),
        **STATUS_BLOCKS[kind],
    })
    return xrd(kind, plural, "database.entigo.com", spec, status)


S = lambda: field("string")
STATUS_BLOCKS = {
    "PostgreSQLInstance": {
        "aws": obj({"instanceType": S(), "finalBackup": field(
                        "string", "The ID the final snapshot receives when the instance is "
                        "deleted, for aws.fromBackup."),
                    "identifier": S(), "resourceId": S(),
                    "engineVersionActual": S(),
                    "latestRestorableTime": field("string", format="date-time"),
                    "status": S()}),
        "gcp": obj({"instanceType": S(), "id": S(), "connectionName": S(),
                    "maintenanceVersion": S()}),
        "oci": obj({"instanceType": S(), "id": S(), "instanceOcpuCount": field("number"),
                    "instanceMemorySizeInGbs": field("integer"), "state": S()}),
        "azure": obj({"instanceType": S(), "id": S()}),
    },
    "MariaDBInstance": {
        "aws": obj({"instanceType": S(), "finalBackup": field(
                        "string", "The ID the final snapshot receives when the instance is "
                        "deleted, for aws.fromBackup."),
                    "identifier": S(), "resourceId": S(),
                    "engineVersionActual": S(),
                    "latestRestorableTime": field("string", format="date-time"),
                    "status": S()}),
    },
}

write("postgresqlinstance", db_instance(
    "PostgreSQLInstance", "pginstances", "PostgreSQL",
    "The major version, such as \"17\". With maintenance.autoMinorVersionUpgrade false, the "
    "full version, such as \"17.7\".", "^[0-9]+(\\.[0-9]+)?$",
    {
        "aws": {
            "instanceType": field("string", "The RDS instance class, such as db.t4g.small.",
                                  default=DEPLOY),
            "parameterGroupName": field("string", "An existing RDS parameter group to use, in "
                                        "place of parameters."),
            "fromBackup": from_backup_field("An RDS snapshot ID to create the instance from."),
        },
        "gcp": {
            "instanceType": field("string", "The Cloud SQL machine tier, such as "
                                  "db-custom-2-7680, which belongs to gcp.edition.",
                                  default=DEPLOY),
            "edition": field("string", "Cloud SQL edition, such as ENTERPRISE.",
                             default=DEPLOY),
            "diskType": field("string", "Cloud SQL disk type, such as PD_SSD.",
                              default=DEPLOY),
            "fromBackup": from_backup_field("A Cloud SQL backup to create the instance from, as "
                                            "<instance>/<backup run ID>."),
        },
        "oci": {
            "instanceType": field("string", "The DB system shape. A .Flex shape needs "
                                  "oci.ocpus and oci.memoryGB.", default=DEPLOY),
            "ocpus": field("number", "Required with a .Flex shape.", default=DEPLOY,
                           minimum=1),
            "memoryGB": field("integer", "Required with a .Flex shape.", default=DEPLOY,
                              minimum=1),
            "fromBackup": from_backup_field("A DB system backup OCID to create the instance "
                                            "from."),
        },
        "azure": {
            "instanceType": field("string", "The flexible server SKU, such as "
                                  "GP_Standard_D2ds_v5.", default=DEPLOY),
            "storageTier": field("string", "The disk tier, such as P10, which sets its IOPS.",
                                 default=DEPLOY),
        },
    }, sc=False, extra_rules=from_backup_rules(["aws", "gcp", "oci"]) + [
        ("!has(self.aws) || !has(self.aws.parameterGroupName) || !has(self.parameters)",
         "aws.parameterGroupName and parameters cannot be used together"),
        ("!has(oldSelf.aws) || !has(oldSelf.aws.parameterGroupName) || !has(self.version) || "
         "!has(oldSelf.version) || "
         "self.version.split('.')[0] == oldSelf.version.split('.')[0] || !has(self.aws) || "
         "!has(self.aws.parameterGroupName) || "
         "self.aws.parameterGroupName != oldSelf.aws.parameterGroupName",
         "a major version change needs a new aws.parameterGroupName, since a parameter group "
         "fits one major version"),
        ("!has(self.version) || self.version.contains('.') == (has(self.maintenance) && "
         "has(self.maintenance.autoMinorVersionUpgrade) && "
         "!self.maintenance.autoMinorVersionUpgrade)",
         "version takes a minor exactly when maintenance.autoMinorVersionUpgrade is false"),
    ], from_backup=True, major=lambda v: f"{v}.split('.')[0]",
    minor_ext="Extended for a minor: AWS."))

MARIADB_MAJOR = lambda v: f"({v}.split('.')[0] + '.' + {v}.split('.')[1])"

write("mariadbinstance", db_instance(
    "MariaDBInstance", "mariadbinstances", "MariaDB",
    "The major version, which has two numbers, such as \"11.4\". With "
    "maintenance.autoMinorVersionUpgrade false, the full version, such as \"11.4.10\".",
    "^[0-9]+\\.[0-9]+(\\.[0-9]+)?$",
    {"aws": {"instanceType": field("string", "The RDS instance class, such as db.t4g.small.",
                                   default=DEPLOY),
             "parameterGroupName": field("string", "An existing RDS parameter group to use, in "
                                         "place of parameters."),
             "fromBackup": from_backup_field("An RDS snapshot ID to create the instance from.")}},
    sc=True, extra_rules=from_backup_rules(["aws"]) + [
        ("!has(self.aws) || !has(self.aws.parameterGroupName) || !has(self.parameters)",
         "aws.parameterGroupName and parameters cannot be used together"),
        ("!has(oldSelf.aws) || !has(oldSelf.aws.parameterGroupName) || !has(self.version) || "
         "!has(oldSelf.version) || " + MARIADB_MAJOR("self.version") + " == "
         + MARIADB_MAJOR("oldSelf.version") + " || !has(self.aws) || "
         "!has(self.aws.parameterGroupName) || "
         "self.aws.parameterGroupName != oldSelf.aws.parameterGroupName",
         "a major version change needs a new aws.parameterGroupName, since a parameter group "
         "fits one major version"),
        ("!has(self.version) || (self.version.split('.').size() == 3) == "
         "(has(self.maintenance) && has(self.maintenance.autoMinorVersionUpgrade) && "
         "!self.maintenance.autoMinorVersionUpgrade)",
         "version takes a minor exactly when maintenance.autoMinorVersionUpgrade is false"),
    ], from_backup=True, pitr=False, major=MARIADB_MAJOR))

# --- Kinds inside a database instance: users and databases ---------------------------------

NAME_RULE = ("(has(self.name) ? self.name : '') == (has(oldSelf.name) ? oldSelf.name : '')",
             "name is immutable")


def engine_name(what):
    return field("string", f"The {what}'s name in the engine, which may hold what a Kubernetes "
                 "name cannot. Unset, metadata.name. Set at creation only.")


write("postgresqluser", xrd(
    "PostgreSQLUser", "postgresqlusers", "database.entigo.com",
    obj({
        "name": engine_name("role"),
        "instanceRef": parent_ref("PostgreSQLInstance", "instanceRef"),
        "secretName": secret_name("<instance>-<user>"),
        "login": field("boolean", "The role can log in.", default="fixed, true"),
        "createDb": field("boolean", "The role can create databases.", default="fixed, false"),
        "createRole": field("boolean", "The role can create roles.", default="fixed, false"),
        "inherit": field("boolean", "The role inherits the privileges of roles it is granted.",
                         default="fixed, true"),
        "grant": obj({"roles": array(field("string"), "Roles granted to this role.")}),
    }, rules=[NAME_RULE]),
    obj({"connectionSecret": field("string", "The Secret with this user's login (rule 6).")}),
    required_spec=["instanceRef"]))

write("postgresqldatabase", xrd(
    "PostgreSQLDatabase", "postgresqldatabases", "database.entigo.com",
    obj({
        "name": engine_name("database"),
        "instanceRef": parent_ref("PostgreSQLInstance", "instanceRef"),
        "owner": field("string", "The owning role's name in PostgreSQL."),
        "encoding": field("string", "The character set encoding, such as UTF8."),
        "lcCType": field("string", "The character classification (LC_CTYPE)."),
        "lcCollate": field("string", "The collation order (LC_COLLATE)."),
        "dbTemplate": field("string", "The template database to create it from."),
        "extensions": array(field("string", enum=["postgis", "postgis_topology", "fuzzystrmatch",
                                                  "postgis_tiger_geocoder", "uuid-ossp",
                                                  "btree_gist"]),
                            "PostgreSQL extensions to create in the database."),
        "extensionConfig": {"type": "object", "description": desc(
            "Per-extension settings, by extension name."),
                            "additionalProperties": obj({"schema": field(
                                "string", "The schema to create the extension's objects in.")})},
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
    }, rules=[NAME_RULE]),
    obj({}),
    required_spec=["instanceRef", "owner"]))

write("mariadbuser", xrd(
    "MariaDBUser", "mariadbusers", "database.entigo.com",
    obj({
        "name": engine_name("user"),
        "instanceRef": parent_ref("MariaDBInstance", "instanceRef"),
        "secretName": secret_name("<instance>-<user>"),
        "databases": array(obj({
            "databaseRef": obj({"name": field("string", "The MariaDBDatabase.", maxLength=253),
                                "namespace": field("string", "The MariaDBDatabase's namespace, "
                                                   "which must be in this object's Zone (rule "
                                                   "11). Unset, this object's namespace.",
                                                   maxLength=63)},
                               "The database the privileges apply to.", required=["name"]),
            "table": field("string", "The tables the privileges apply to.", default="fixed, *",
                           maxLength=64),
            "privileges": array(field("string", pattern="^([A-Z0-9_ ]+|[A-Z_ ]+ [(]`[^`]+`(, "
                                      "`[^`]+`)*[)])$", maxLength=256),
                                "Privileges granted, such as SELECT.", minItems=1, maxItems=32),
        }, required=["databaseRef", "privileges"]),
            "The databases this user may use on the instance, each with its privileges.",
            minItems=1, maxItems=16,
            **{"x-kubernetes-validations": [{
                "rule": "self.all(a, self.exists_one(b, b.databaseRef == a.databaseRef && "
                        "(has(b.table) ? b.table : '*') == (has(a.table) ? a.table : '*')))",
                "message": "each database and table appears once"}]}),
        "maxConnectionsPerHour": field("integer", "Connections allowed per hour; 0 is unlimited.",
                                       default="fixed, 0", minimum=0),
        "maxQueriesPerHour": field("integer", "Queries allowed per hour; 0 is unlimited.", default="fixed, 0",
                                   minimum=0),
        "maxUpdatesPerHour": field("integer", "Updates allowed per hour; 0 is unlimited.", default="fixed, 0",
                                   minimum=0),
        "maxUserConnections": field("integer", "Connections allowed at once; 0 means the server's max_connections.", default="fixed, 0",
                                    minimum=0),
        "grant": obj({"users": array(field("string"), "Users granted to this user.")}),
    }, rules=[NAME_RULE]),
    obj({"connectionSecret": field("string", "The Secret with this user's login (rule 6).")}),
    required_spec=["instanceRef", "databases"]))

write("mariadbdatabase", xrd(
    "MariaDBDatabase", "mariadbdatabases", "database.entigo.com",
    obj({
        "name": engine_name("database"),
        "instanceRef": parent_ref("MariaDBInstance", "instanceRef"),
        "defaultCharacterSet": field("string", "The default character set."),
        "defaultCollation": field("string", "The default collation."),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
    }, rules=[NAME_RULE]),
    obj({}),
    required_spec=["instanceRef"]))

# --- Valkey -------------------------------------------------------------------------------

write("valkeyinstance", xrd(
    "ValkeyInstance", "valkeyinstances", "database.entigo.com",
    resource_spec({
        "version": field("string", "Valkey's major and minor, such as \"8.1\", and the floor "
                         "after: raising it upgrades the cache, lowering it is rejected. With "
                         "maintenance.autoMinorVersionUpgrade, the cloud can move the cache "
                         "past it; the status shows the running version.",
                         default=DEPLOY, pattern="^[0-9]+\\.[0-9]+$"),
        "highAvailability": field("boolean", "A replica in another zone, with automatic "
                                  "failover. Needs replicas of at least 1.", default=DEPLOY),
        "replicas": field("integer", "Read replicas besides the primary, within the cloud's "
                          "own limit (rule 10).", default=DEPLOY, minimum=0),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenance": obj({
            "window": window("Extended: AWS, GCP."),
            "autoMinorVersionUpgrade": field("boolean", "Allow the cloud to upgrade to newer minor "
                                             "versions on its schedule. false keeps the cache "
                                             "at version.", "Extended for false: AWS.",
                                             DEPLOY),
        }),
        "backup": obj({
            "retentionDays": field("integer", "Days automated backups are kept, within the "
                                   "cloud's own range (rule 10). 0 turns automated backups "
                                   "off.", default=DEPLOY, minimum=0),
            "startHourUTC": field("integer", "The hour, in UTC, the daily backup starts.",
                                  minimum=0, maximum=23),
        }, "Automated daily backups.", "Extended: AWS, GCP."),
        "secretName": secret_name("<instance>-credentials"),
        "parameters": string_map("Valkey settings, by name. Keys outside the cloud's allowlist "
                                 "are rejected."),
    }, {
        "aws": {"instanceType": field("string", "The ElastiCache node type, such as "
                                      "cache.t4g.small.", default=DEPLOY),
                "parameterGroupName": field("string", "An existing ElastiCache parameter group "
                                            "to use, in place of parameters.")},
        "gcp": {"instanceType": field("string", "The Memorystore node type.", default=DEPLOY)},
        "oci": {"memoryGB": field("integer", "Memory per node, within OCI's own range "
                                  "(rule 10).", default=DEPLOY, minimum=1)},
    }, "cache", [
        ("!has(self.highAvailability) || !self.highAvailability || "
         "(has(self.replicas) && self.replicas >= 1)",
         "highAvailability needs replicas of at least 1"),
        ("!has(self.version) || !has(oldSelf.version) || "
         "int(self.version.split('.')[0]) > int(oldSelf.version.split('.')[0]) || "
         "(int(self.version.split('.')[0]) == int(oldSelf.version.split('.')[0]) && "
         "int(self.version.split('.')[1]) >= int(oldSelf.version.split('.')[1]))",
         "version cannot decrease"),
        ("!has(self.aws) || !has(self.aws.parameterGroupName) || !has(self.parameters)",
         "aws.parameterGroupName and parameters cannot be used together"),
        ("!has(oldSelf.aws) || !has(oldSelf.aws.parameterGroupName) || !has(self.version) || "
         "!has(oldSelf.version) || "
         "self.version.split('.')[0] == oldSelf.version.split('.')[0] || !has(self.aws) || "
         "!has(self.aws.parameterGroupName) || "
         "self.aws.parameterGroupName != oldSelf.aws.parameterGroupName",
         "a major version change needs a new aws.parameterGroupName, since a parameter group "
         "fits one major version"),
    ]),
    obj({
        "version": field("string"),
        "highAvailability": field("boolean"),
        "replicas": field("integer"),
        "endpoint": endpoint("The primary's address."),
        "readerEndpoint": endpoint("The address that spreads reads over the replicas."),
        "connectionSecret": field("string", "The Secret <instance>-credentials (rule 6)."),
        "aws": obj({"instanceType": S(), "finalBackup": field(
            "string", "The ID the final snapshot receives when the cache is deleted."),
            "id": S(), "engineVersionActual": S()}),
        "gcp": obj({"instanceType": S(), "id": S()}),
        "oci": obj({"memoryGB": field("integer"), "id": S(), "state": S(),
                    "softwareVersion": S()}),
    })))

# --- Kafka --------------------------------------------------------------------------------

write("kafkacluster", xrd(
    "KafkaCluster", "kafkaclusters", "queue.entigo.com",
    resource_spec({
        "version": field("string", "The Kafka version, such as \"3.9\". Each cloud accepts "
                         "the versions it offers.", default=DEPLOY),
        "brokers": field("integer", "The number of brokers.",
                         "Extended: AWS, OCI. GCP chooses its own.", DEPLOY, minimum=1),
        "storage": obj({"sizeGB": field("integer", "Disk per broker, in GB.", default=DEPLOY,
                                        minimum=1)}),
        "highAvailability": field("boolean", "Survive the loss of one zone.",
                                  "Extended for false: OCI.", DEPLOY),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "parameters": string_map("Broker settings, by name. Keys outside the cloud's "
                                 "allowlist are rejected."),
    }, {
        "aws": {"instanceType": field("string", "The MSK broker instance type, such as "
                                      "kafka.m7g.large.", default=DEPLOY)},
        "gcp": {
            "vcpus": field("integer", "The whole cluster's vCPUs.", default=DEPLOY, minimum=1),
            "memoryGB": field("integer", "The whole cluster's memory, within GCP's range per vCPU (rule 10).",
                              default=DEPLOY, minimum=1),
        },
        "oci": {"instanceType": field("string", "The broker shape. A .Flex shape needs "
                                      "oci.ocpus.", default=DEPLOY),
                "ocpus": field("number", "OCPUs per broker. Required with a .Flex shape.",
                               default=DEPLOY, minimum=1)},
    }, "Kafka cluster"),
    obj({
        "version": field("string"),
        "brokers": field("integer"),
        "storage": obj({"sizeGB": field("integer")}),
        "highAvailability": field("boolean"),
        "bootstrapServers": field("string", "The listener applications connect to, as host:port pairs."),
        "aws": obj({"instanceType": S(), "arn": S(), "kafkaVersion": S()}),
        "gcp": obj({"vcpus": field("integer"), "memoryGB": field("integer"), "id": S()}),
        "oci": obj({"instanceType": S(), "id": S(), "state": S()}),
    })))

CLUSTER_REF = parent_ref("KafkaCluster", "clusterRef")

write("kafkauser", xrd(
    "KafkaUser", "kafkausers", "queue.entigo.com",
    obj({
        "clusterRef": CLUSTER_REF,
        "secretName": secret_name("<cluster>-<user>"),
        "consumerGroups": array(field("string"), "Consumer groups the user may join."),
        "acls": array(obj({
            "topic": field("string"),
            "operation": field("string", enum=["Read", "Write", "Create", "Delete", "Alter",
                                               "Describe", "DescribeConfigs", "AlterConfigs",
                                               "All"]),
        }, required=["topic", "operation"]), "Kafka ACLs that allow this user an operation "
            "on a topic."),
    }),
    obj({"connectionSecret": field("string", "The Secret <cluster>-<user> (rule 6).")}),
    required_spec=["clusterRef"]))

write("kafkatopic", xrd(
    "KafkaTopic", "kafkatopics", "queue.entigo.com",
    obj({
        "clusterRef": CLUSTER_REF,
        "name": field("string", "The Kafka topic's name. Unset, metadata.name. The function "
                      "rejects a name another KafkaTopic on the same cluster already claims; "
                      "the older one keeps it. Kafka cannot rename a topic, so it is immutable.",
                      minLength=1, maxLength=249, pattern="^[a-zA-Z0-9._-]+$",
                      **{"x-kubernetes-validations": IMMUTABLE_MSG("name")}),
        "partitions": field("integer", "Kafka cannot remove partitions, so this only grows.",
                            default=DEPLOY, minimum=1,
                            **{"x-kubernetes-validations": [{"rule": "self >= oldSelf",
                                                             "message": "partitions cannot "
                                                             "decrease"}]}),
        "replicationFactor": field("integer", "Copies of each partition, at most the broker "
                                   "count.", default=DEPLOY, minimum=1),
        "parameters": string_map("Topic settings, by name, such as retention.ms."),
    }),
    obj({"name": field("string", "The Kafka topic's name."), "partitions": field("integer"),
         "replicationFactor": field("integer")}),
    required_spec=["clusterRef"]))

# --- RabbitMQ -----------------------------------------------------------------------------

write("rabbitmqbroker", xrd(
    "RabbitMQBroker", "rabbitmqbrokers", "mq.entigo.com",
    resource_spec({
        "version": field("string", "RabbitMQ's release line, such as \"4.2\". The cloud "
                         "manages the patch.", default=DEPLOY, pattern="^[0-9]+\\.[0-9]+$"),
        "highAvailability": field("boolean", "A three-node cluster across zones.",
                                  default=DEPLOY),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenance": obj({
            "window": window(),
        }),
        "secretName": secret_name("<broker>-credentials"),
        "parameters": string_map("rabbitmq.conf settings, by name. Keys outside the cloud's "
                                 "allowlist are rejected."),
    }, {"aws": {"instanceType": field("string", "The Amazon MQ host instance type, such as "
                                      "mq.m7g.medium.", default=DEPLOY),
                "publiclyAccessible": field(
        "boolean", "Reachable from the internet, without the platform's security group. Amazon "
        "MQ replaces the broker to change it, so it is set at creation only.",
        default="fixed, false",
        **{"x-kubernetes-validations": IMMUTABLE_MSG("aws.publiclyAccessible")})}}, "broker"),
    obj({
        "version": field("string"),
        "highAvailability": field("boolean"),
        "endpoint": endpoint("The AMQPS address."),
        "managementEndpoint": field("string", "The management console URL."),
        "connectionSecret": field("string", "The Secret <broker>-credentials (rule 6)."),
        "aws": obj({"instanceType": S(), "id": S(), "engineVersion": S()}),
    })))

BROKER_REF = parent_ref("RabbitMQBroker", "brokerRef")

write("rabbitmqvirtualhost", xrd(
    "RabbitMQVirtualHost", "rabbitmqvirtualhosts", "mq.entigo.com",
    obj({
        "name": engine_name("virtual host"),
        "brokerRef": BROKER_REF,
        "deletionProtection": field("boolean", "Deleting the object, and with it the virtual "
                                    "host's queues and messages, is denied while true.",
                                    default="fixed, true"),
    }, rules=[NAME_RULE]),
    obj({"name": field("string", "The virtual host's name in RabbitMQ.")}),
    required_spec=["brokerRef"]))

write("rabbitmquser", xrd(
    "RabbitMQUser", "rabbitmqusers", "mq.entigo.com",
    obj({
        "name": engine_name("user"),
        "brokerRef": BROKER_REF,
        "secretName": secret_name("<broker>-<user>"),
        "permissions": array(obj({
            "virtualHost": field("string", "The virtual host's name in RabbitMQ."),
            "configure": field("string", "A regular expression over the names of queues and "
                               "exchanges the user may create and delete. Unset or empty "
                               "allows none."),
            "write": field("string", "A regular expression over the names the user may "
                           "publish to. Unset or empty allows none."),
            "read": field("string", "A regular expression over the names the user may "
                          "consume from. Unset or empty allows none."),
        }, required=["virtualHost"]), "RabbitMQ permissions, one entry per virtual host.",
            **{"x-kubernetes-list-type": "map", "x-kubernetes-list-map-keys": ["virtualHost"]}),
    }, rules=[NAME_RULE]),
    obj({"connectionSecret": field("string", "The Secret <broker>-<user> (rule 6).")}),
    required_spec=["brokerRef"]))

# --- Bucket -------------------------------------------------------------------------------

write("bucket", xrd(
    "Bucket", "buckets", "storage.entigo.com",
    resource_spec({
        "versioning": field("boolean", "Keep replaced and deleted objects as versions.",
                            default=DEPLOY),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "lifecycleRules": array(obj({
            "prefix": field("string", "Objects whose key starts with this. Unset, every "
                            "object."),
            "expireAfterDays": field("integer", "Delete objects this old.", minimum=1),
            "deleteNoncurrentAfterDays": field("integer", "Delete replaced versions this old.",
                                               minimum=1),
            "transitions": array(obj({
                "afterDays": field("integer", minimum=1),
                "tier": field("string", "One of the two cold tiers every cloud has.",
                              enum=["Infrequent", "Archive"]),
            }, required=["afterDays", "tier"]), "Move objects to a colder tier."),
        })),
        "retention": obj({
            "days": field("integer", minimum=1),
            "locked": field("boolean", "true cannot be undone on any cloud.",
                            **{"x-kubernetes-validations": [{
                                "rule": "!oldSelf || self",
                                "message": "a locked retention cannot be unlocked"}]}),
        }, "Objects cannot be changed or deleted for this many days.", required=["days"]),
        "cors": array(obj({
            "origins": array(field("string")),
            "methods": array(field("string", enum=["GET", "PUT", "POST", "DELETE", "HEAD"])),
            "maxAgeSeconds": field("integer", minimum=0),
        }, required=["origins", "methods"]), "Cross-origin requests browsers may make.",
            "Extended: AWS, GCP, Azure."),
    }, {
        "aws": None,
        "gcp": {"softDeleteDays": field("integer", "Days deleted objects can be recovered.",
                                        default=DEPLOY, minimum=0)},
        "oci": None,
        "azure": {"softDeleteDays": field("integer", "Days deleted objects can be recovered.",
                                          default=DEPLOY, minimum=0)},
    }, "bucket", [
        ("!has(oldSelf.retention) || !has(oldSelf.retention.locked) || !oldSelf.retention.locked "
         "|| (has(self.retention) && has(self.retention.locked) && self.retention.locked)",
         "a locked retention cannot be unlocked or removed"),
    ]),
    obj({
        "url": field("string"),
        "bucketName": field("string"),
        "region": field("string"),
        "aws": obj({"arn": S()}),
        "gcp": obj({"selfLink": S()}),
        "oci": obj({"namespace": S(), "retentionLockedFrom": field(
            "string", "When a locked retention takes effect.", format="date-time")}),
        "azure": obj({"accountId": S(), "containerId": S()}),
    })))

# --- ArtifactRepository ---------------------------------------------------------------------

write("artifactrepository", xrd(
    "ArtifactRepository", "artifactrepositories", "artifact.entigo.com",
    resource_spec({
        "name": field("string", "The image name. Unset, the object's name. Set at creation "
                      "only."),
        "path": field("string", "A prefix between the namespace and the name: the image path "
                      "is <namespace>/<path>/<name>, or <namespace>/<name> unset. Set at "
                      "creation only."),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "immutableTags": field("boolean", "A pushed tag cannot be pushed again.",
                               "Extended: AWS, GCP, OCI.", DEPLOY),
        "lifecycleRules": array(obj({
            "tagPrefixes": array(field("string"), "Images with a tag starting with one of "
                                 "these."),
            "tagPatterns": array(field("string"), "Images with a tag matching one of these "
                                 "wildcards.", "Extended: AWS."),
            "untagged": field("boolean", "Images without a tag."),
            "keepCount": field("integer", "Keep this many newest images. With tagPrefixes or "
                               "untagged, only on AWS; GCP keeps the newest only of all images.",
                               "Extended with a selector: AWS.", minimum=1),
            "expireAfterDays": field("integer", "Delete images this long after their push.",
                                     minimum=1),
        }, "One selector, or none for every image, and one bound.", rules=[
            ("(has(self.tagPrefixes) ? 1 : 0) + (has(self.tagPatterns) ? 1 : 0) + "
             "(has(self.untagged) ? 1 : 0) <= 1", "at most one selector"),
            ("has(self.keepCount) != has(self.expireAfterDays)",
             "exactly one of keepCount and expireAfterDays"),
        ]), "Delete old images. Set, it replaces the deployment's rules.", "Extended: AWS, GCP.",
           default=DEPLOY),
    }, {"aws": None, "gcp": None, "oci": None, "azure": None}, "repository", [
        ("(has(self.name) ? self.name : '') == (has(oldSelf.name) ? oldSelf.name : '')",
         "name is immutable"),
        ("(has(self.path) ? self.path : '') == (has(oldSelf.path) ? oldSelf.path : '')",
         "path is immutable"),
    ], observe_ext="Extended: AWS, GCP, OCI. An Azure repository has no resource of its own."),
    obj({
        "uri": field("string", "The image URI, such as "
                     "123456789012.dkr.ecr.eu-north-1.amazonaws.com/shop/orders-api."),
        "aws": obj({"arn": S()}),
        "gcp": obj({"id": S()}),
        "oci": obj({"id": S()}),
        "azure": obj({"registryId": S()}),
    })))
