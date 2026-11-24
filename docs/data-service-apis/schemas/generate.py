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
        lines += textwrap.wrap(ext, 80)
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


def parent_ref(kind, name, today=None):
    props = {"name": field("string", f"The {kind}."),
             "namespace": field("string", f"The {kind}'s namespace, which must be in this "
                                "object's Zone (principle 11). Unset, this object's "
                                "namespace.")}
    rules = [("self == oldSelf", f"{name} is immutable")]
    if today:
        props["kind"] = field("string", f"Set to {today} while the parent is still today's "
                              f"{today}, so this object keeps today's behaviour; unset, a {kind}. "
                              f"It can be removed once, when the parent moves to a {kind}.",
                              enum=[today])
        rules = [("self == oldSelf || (has(oldSelf.kind) && !has(self.kind))",
                  f"{name} is immutable, except that kind can be removed once")]
    return obj(props, f"The {kind} this object lives in.", required=["name"], rules=rules)


def secret_name(default):
    return field("string", "The name of the connection Secret (principle 6). The platform never "
                 "overwrites a Secret that another object or person owns; set this to avoid one.",
                 default=f"fixed, {default}")


def fixed(name, message=None):
    """A field that cannot be added, removed or changed after creation. A rule on the field itself
    runs only when the old and new values both exist, so this rule sits on its parent."""
    return (f"has(self.{name}) == has(oldSelf.{name}) && "
            f"(!has(self.{name}) || self.{name} == oldSelf.{name})",
            message or f"{name} is immutable")


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


def window(ext=None, sc=False):
    return obj({"day": DAY, "hourUTC": HOUR},
               "The weekly hour, in UTC, in which the cloud applies updates.", ext, None, sc,
               required=["day", "hourUTC"])


S = lambda: field("string")

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
        "clusterRef": parent_ref("PostgreSQLCluster", "clusterRef", "PostgreSQLInstance"),
        "secretName": secret_name("<instance>-<user>"),
        "login": field("boolean", "The role can log in.", default="fixed, true"),
        "createDb": field("boolean", "The role can create databases.", default="fixed, false"),
        "createRole": field("boolean", "The role can create roles.", default="fixed, false"),
        "inherit": field("boolean", "The role inherits the privileges of roles it is granted.",
                         default="fixed, true"),
        "grant": obj({"roles": array(field("string"), "Roles granted to this role.")}),
    }, rules=[NAME_RULE]),
    obj({"connectionSecret": field("string", "The Secret with this user's login (principle 6).")}),
    required_spec=["clusterRef"]))

write("postgresqldatabase", xrd(
    "PostgreSQLDatabase", "postgresqldatabases", "database.entigo.com",
    obj({
        "name": engine_name("database"),
        "clusterRef": parent_ref("PostgreSQLCluster", "clusterRef", "PostgreSQLInstance"),
        "owner": field("string", "The owning role's name in PostgreSQL."),
        "encoding": field("string", "The character set encoding, such as UTF8."),
        "lcCType": field("string", "The character classification (LC_CTYPE)."),
        "lcCollate": field("string", "The collation order (LC_COLLATE)."),
        "dbTemplate": field("string", "The template database to create it from."),
        "extensions": array(field("string", enum=["postgis", "postgis_topology", "fuzzystrmatch",
                                                  "postgis_tiger_geocoder", "uuid-ossp",
                                                  "btree_gist", "vector"]),
                            "PostgreSQL extensions to create in the database."),
        "extensionConfig": {"type": "object", "description": desc(
            "Per-extension settings, by extension name."),
                            "additionalProperties": obj({"schema": field(
                                "string", "The schema to create the extension's objects in.")})},
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
    }, rules=[NAME_RULE]),
    obj({}),
    required_spec=["clusterRef", "owner"]))

write("mariadbuser", xrd(
    "MariaDBUser", "mariadbusers", "database.entigo.com",
    obj({
        "name": engine_name("user"),
        "serverRef": parent_ref("MariaDBServer", "serverRef", "MariaDBInstance"),
        "secretName": secret_name("<instance>-<user>"),
        "databases": array(obj({
            "databaseRef": obj({"name": field("string", "The MariaDBDatabase.", maxLength=253),
                                "namespace": field("string", "The MariaDBDatabase's namespace, "
                                                   "which must be in this object's Zone "
                                                   "(principle 11). Unset, this object's namespace.",
                                                   maxLength=63)},
                               "The database the privileges apply to.", required=["name"]),
            "table": field("string", "The tables the privileges apply to.", default="fixed, *",
                           maxLength=64),
            "privileges": array(field("string", pattern="^([A-Z0-9_ ]+|[A-Z_ ]+ [(]`[^`]+`(, "
                                      "`[^`]+`)*[)])$", maxLength=256),
                                "Privileges granted, such as SELECT.", minItems=1, maxItems=32),
        }, required=["databaseRef", "privileges"]),
            "The databases this user may use on the server, each with its privileges.",
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
        "maxUserConnections": field("integer", "Connections allowed at once; 0 is no limit of the user's own, so only the server's max_connections applies.", default="fixed, 0",
                                    minimum=0),
    }, rules=[NAME_RULE]),
    obj({"connectionSecret": field("string", "The Secret with this user's login (principle 6).")}),
    required_spec=["serverRef", "databases"]))

write("mariadbdatabase", xrd(
    "MariaDBDatabase", "mariadbdatabases", "database.entigo.com",
    obj({
        "name": engine_name("database"),
        "serverRef": parent_ref("MariaDBServer", "serverRef", "MariaDBInstance"),
        "defaultCharacterSet": field("string", "The default character set."),
        "defaultCollation": field("string", "The default collation."),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
    }, rules=[NAME_RULE]),
    obj({}),
    required_spec=["serverRef"]))

# --- Kafka --------------------------------------------------------------------------------

CLUSTER_REF = parent_ref("KafkaCluster", "clusterRef")
PATTERN_TYPE = field("string", "Kafka's ACL pattern type: Literal matches the name exactly, "
                     "Prefixed every name that starts with it.", default="fixed, Literal",
                     enum=["Literal", "Prefixed"])

write("kafkauser", xrd(
    "KafkaUser", "kafkausers", "queue.entigo.com",
    obj({
        "clusterRef": parent_ref("KafkaCluster", "clusterRef", "MSK"),
        "secretName": secret_name("<cluster>-<user>"),
        "consumerGroups": array(obj({
            "name": field("string", "The group's name, or with patternType Prefixed, the start of "
                          "every name it covers."),
            "patternType": PATTERN_TYPE,
        }, required=["name"]), "Consumer groups the user may join."),
        "acls": array(obj({
            "topic": field("string", "The topic's name, or with patternType Prefixed, the start of "
                           "every name it covers."),
            "patternType": PATTERN_TYPE,
            "operation": field("string", enum=["Read", "Write", "Create", "Delete", "Alter",
                                               "Describe", "DescribeConfigs", "AlterConfigs",
                                               "All"]),
        }, required=["topic", "operation"]), "Kafka ACLs that allow this user an operation "
            "on a topic."),
    }),
    obj({"connectionSecret": field("string", "The Secret <cluster>-<user> (principle 6).")}),
    required_spec=["clusterRef"]))

TOPIC_DEFAULT = "the topicTemplate of the cluster's KafkaClass"

write("kafkatopic", xrd(
    "KafkaTopic", "kafkatopics", "queue.entigo.com",
    obj({
        "clusterRef": CLUSTER_REF,
        "name": field("string", "The Kafka topic's name. Unset, metadata.name. The function "
                      "rejects a name another KafkaTopic on the same cluster already claims; "
                      "the older one keeps it. Kafka cannot rename a topic, so it is immutable.",
                      minLength=1, maxLength=249, pattern="^[a-zA-Z0-9._-]+$"),
        "partitions": field("integer", "Kafka cannot remove partitions: a value below the topic's "
                            "count is ignored, and the status shows the real count.",
                            default=TOPIC_DEFAULT, minimum=1),
        "replicationFactor": field("integer", "Copies of each partition, at most the broker "
                                   "count.", default=TOPIC_DEFAULT, minimum=1),
        "parameters": string_map("Topic settings, by name, such as retention.ms."),
    }, rules=[fixed("name")]),
    obj({"name": field("string", "The Kafka topic's name."), "partitions": field("integer"),
         "replicationFactor": field("integer")}),
    required_spec=["clusterRef"]))

# --- RabbitMQ -----------------------------------------------------------------------------

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
    obj({"connectionSecret": field("string", "The Secret <broker>-<user> (principle 6).")}),
    required_spec=["brokerRef"]))

# --- Bucket -------------------------------------------------------------------------------

# --- ArtifactRepository ---------------------------------------------------------------------

# --- Provider configurations and classes (principles 2, 5 and 8) ---------------------------------



def crd(kind, plural, group, scope, spec, status=None, required_spec=None):
    """A plain CRD, for kinds that are not composed: classes, parameters and provider configurations."""
    props = {"spec": spec}
    if status:
        props["status"] = status
    if required_spec:
        spec["required"] = required_spec
    return {
        "apiVersion": "apiextensions.k8s.io/v1",
        "kind": "CustomResourceDefinition",
        "metadata": {"name": f"{plural}.{group}"},
        "spec": {
            "group": group,
            "scope": scope,
            "names": {"kind": kind, "plural": plural},
            "versions": [{"name": "v1alpha1" if "." in group.split(".entigo")[0] else "v1alpha2",
                          "served": True, "storage": True,
                          "schema": {"openAPIV3Schema": {"type": "object", "properties": props,
                                                         "required": ["spec"]}},
                          **({"subresources": {"status": {}}} if status else {})}],
        },
    }


def entry_fields(text):
    return {
        "apiVersion": field("string", "The provider configuration kind's group and version, such as "
                            "aws.database.entigo.com/v1alpha1.", maxLength=253),
        "kind": field("string", "The provider configuration kind, such as PostgreSQLRDSConfig.",
                      maxLength=63),
        "spec": {"type": "object", "x-kubernetes-preserve-unknown-fields": True,
                 "description": desc(text)},
        "ref": obj({"name": field("string", "An object of this kind in the same namespace.")},
                   "The settings in a separate object of this kind, instead of spec.",
                   required=["name"]),
    }


def provider_configs():
    """The open list of provider configurations (principle 2): one entry per kind, inline or by reference."""
    return array(
        obj(entry_fields("The settings, checked by the implementation that applies them "
                         "(principle 10)."), required=["apiVersion", "kind"],
            rules=[("has(self.spec) != has(self.ref)", "an entry holds either spec or ref")]),
        "Settings of single implementations, one entry per kind (principle 2). The deployment applies "
        "the entry whose group and kind its class names, whatever its version, and ignores the "
        "others.",
        maxItems=16,
        **{"x-kubernetes-list-type": "map", "x-kubernetes-list-map-keys": ["apiVersion", "kind"],
           "x-kubernetes-validations": [{
               "rule": "self.all(a, self.exists_one(b, b.kind == a.kind && "
                       "b.apiVersion.split('/')[0] == a.apiVersion.split('/')[0]))",
               "message": "one entry per group and kind, whatever its version"}]})


def provider_config(text):
    """One provider configuration, as a class holds it: the kind names the implementation."""
    props = entry_fields("The settings.")
    del props["ref"]
    return obj(props, text, required=["apiVersion", "kind"])


# Appended to a rule that ties fields together: the class's template may have written one of them.
FROM_CLASS = "; a value the object does not set comes from its class's template"


def without_transition_rules(x):
    """A copy without the rules that compare a value with the previous one, such as a disk that
    only grows: they protect a running resource, and a class only holds starting values."""
    if isinstance(x, list):
        return [without_transition_rules(v) for v in x]
    if not isinstance(x, dict):
        return x
    out = {k: without_transition_rules(v) for k, v in x.items()}
    rules = [{**r, "message": r["message"].removesuffix(FROM_CLASS)}
             for r in out.get("x-kubernetes-validations", []) if "oldSelf" not in r["rule"]]
    if "x-kubernetes-validations" in out:
        if rules:
            out["x-kubernetes-validations"] = rules
        else:
            del out["x-kubernetes-validations"]
    return out


def parameters_refs(kind, what, cls=False):
    """An ordered list of references to shared engine settings, merged key by key, later winning."""
    ref = {"name": field("string", maxLength=253)}
    if cls:
        ref["namespace"] = field("string", maxLength=63)
    where = ("beneath every object of this class. They are never copied into the object: the "
             "platform merges the object's parametersRefs and parameters over them, key by key, "
             "at every reconcile.") if cls else ("in this namespace, merged after the class's.")
    return array(obj(ref, required=list(ref)),
                 f"{what}: {kind} objects {where} The list merges in order, key by key, a later "
                 "entry winning; a referenced object names no other.",
                 maxItems=8, **{"x-kubernetes-list-type": "map",
                                "x-kubernetes-list-map-keys": list(ref)})


def class_crd(kind, plural, group, common, extra_rules=(), extra=None, parameters=None):
    """A cluster-scoped class (principle 5): template defaults, enforced values, the engine's base
    parameters, and in extra, defaults for the kinds inside an object of the class."""
    common = {k: v for k, v in without_transition_rules(common).items() if k != "parametersRefs"}
    part = lambda text: obj({**common, "providerConfig": provider_config(
        "The provider configuration: its kind names the implementation this class uses.")}, text)
    if parameters:
        extra = {"parametersRefs": parameters_refs(parameters, "The deployment's base settings",
                                                   cls=True), **(extra or {})}
    same = ("!has(self.template) || !has(self.enforced) || !has(self.template.providerConfig) || "
            "!has(self.enforced.providerConfig) || "
            "(self.template.providerConfig.apiVersion == self.enforced.providerConfig.apiVersion "
            "&& self.template.providerConfig.kind == self.enforced.providerConfig.kind)",
            "template and enforced name the same provider configuration kind")
    spec = obj({
        "template": part("Defaults, written into an object when it is created; the object may "
                         "change them."),
        "enforced": part("Applied from creation and at each class sync; an object that sets a "
                         "different value is refused when created or synced."),
        **(extra or {}),
    }, rules=[same, *extra_rules])
    return crd(kind, plural, group, "Cluster", spec)


def parameters_crd(kind, plural, group, engine):
    return crd(kind, plural, group, "Namespaced", obj({
        "parameters": string_map(f"{engine} settings, by name. Keys outside the implementation's "
                                 "allowlist are rejected by the cloud (principle 10).")},
        rules=[]), required_spec=["parameters"])


def provider_config_crd(kind, plural, group, product, settings, status, rules=()):
    """A provider configuration kind: one implementation's settings, and its status.provider shape."""
    return crd(kind, plural, group, "Namespaced", obj({
        "id": field("string", f"The {product} ID of an existing resource to observe (principle 9). "
                    "Set with observe; it stays when observe is removed, and the object then owns "
                    "the resource."),
        **settings}, f"Settings for {product} (principle 2).",
        rules=[fixed("id"), *[fixed(k, FIXED_SETTINGS[k]) for k in settings if k in FIXED_SETTINGS],
               *rules]),
        obj(status, f"The shape of status.provider for {product} (principle 4)."))


FIXED_SETTINGS = {"fromBackup": "fromBackup is set at creation only",
                  "publiclyAccessible": "publiclyAccessible is immutable"}


def from_backup(text):
    return field("string", text + " Set at creation only; not with restore.")


# --- PostgreSQL ------------------------------------------------------------------------------

DEFAULT = "the class's template"


def db_common(engine, version_text, version_pattern):
    """A database's common fields (principle 1), grouped as principle 13 says."""
    return {
        "version": field("string", version_text, default=DEFAULT, pattern=version_pattern),
        "storage": obj({
            "sizeGB": field("integer", "Disk size in GB at creation, and its floor after: raising "
                            "it grows the disk, and a value below the real size is ignored. The "
                            "disk can be larger: it is rounded up to a size the implementation "
                            "accepts, and autoGrow can take it further. The status shows the real "
                            "size.", default=DEFAULT, minimum=1),
            "autoGrow": field("boolean", "Grow the disk as it fills.", default="fixed, true"),
            "maxSizeGB": field("integer", "The size autoGrow stops at.", default=DEFAULT,
                               minimum=1),
            "iops": field("integer", "Provisioned IOPS.", minimum=1),
            "throughputMBps": field("integer", "Provisioned throughput in MB/s.", minimum=1),
        }, rules=[("!has(self.maxSizeGB) || !has(self.sizeGB) || self.maxSizeGB >= self.sizeGB",
                   "maxSizeGB must be at least sizeGB" + FROM_CLASS)]),
        "highAvailability": field("boolean", "A standby in another zone, with automatic "
                                  "failover. Refused where the implementation does not support "
                                  "HighAvailability (principle 1).", default=DEFAULT),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenanceWindow": window(),
        "autoMinorVersionUpgrade": field("boolean", "Apply new minor versions in the "
                                         "maintenance window. false keeps the minor written in "
                                         "version.", default=DEFAULT),
        "allowMajorVersionUpgrade": field("boolean", "Allow version to change to a later major.",
                                          default="fixed, false"),
        "backup": obj({
            "retentionDays": field("integer", "Days automated backups are kept, within the "
                                   "implementation's own range (principle 10). 0 turns automated "
                                   "backups and point-in-time restore off.", default=DEFAULT,
                                   minimum=0),
            "pointInTimeDays": field("integer", "Days back restore.pointInTime can reach.",
                                     default="fixed, the lower of retentionDays and the "
                                     "implementation's maximum; none with retentionDays 0",
                                     minimum=1),
            "startHourUTC": field("integer", "The hour, in UTC, the daily backup starts.",
                                  minimum=0, maximum=23),
        }, rules=[("!has(self.pointInTimeDays) || !has(self.retentionDays) || "
                   "self.retentionDays > 0",
                   "pointInTimeDays needs backups, which retentionDays 0 disables" + FROM_CLASS)]),
        "parametersRefs": parameters_refs(f"{engine}Parameters", f"Reusable {engine} settings"),
        "parameters": string_map(f"{engine} settings for this object only, merged after "
                                 "parametersRefs."),
        "parameterApplyMethod": field("string", "The moment changed parameters take effect. Immediate "
                                      "restarts the instance where a setting needs it.",
                                      default="fixed, Immediate",
                                      enum=["Immediate", "NextRestart"]),
    }


def class_spec(class_kind, common, tail, observed, rules):
    """The spec of a kind with a class (principle 5): className, observe, the common fields, the
    kind's own fields in tail, and the provider configurations (principle 2)."""
    return obj({
        "className": field("string", f"The {class_kind}: the deployment's defaults, enforced "
                           "values and implementation (principle 5). Unset, the default class, written "
                           "at creation; without one, the object is standalone. A standalone object "
                           "may add it once; it is never changed or removed."),
        "observe": field("boolean", "true observes an existing resource, named by the id in its "
                         "provider configuration entry, and never changes or deletes it. Every other "
                         "common field is then rejected (principle 9). Set at creation only; removing "
                         "it, once, takes ownership of the resource."),
        **common,
        **tail,
        "providerConfigs": provider_configs(),
    }, rules=[
        ("!has(oldSelf.className) || "
         "(has(self.className) && self.className == oldSelf.className)",
         "className can be added once, never changed or removed"),
        ("!(has(self.observe) && self.observe) || (has(oldSelf.observe) && oldSelf.observe)",
         "observe can be removed but never set after creation"),
        ("!(has(self.observe) && self.observe) || (" + " && ".join(
            f"!has(self.{k})" for k in [*common, *observed]) + ")",
         "an observed resource takes no other common field"),
        *rules,
    ])


def db_spec(kind, class_kind, common, same_major, has_minor):
    """A database's spec: its class, observe, the common fields, restore and provider configurations."""
    return class_spec(class_kind, common, {
        "restore": obj({
            "fromInstance": field("string", f"A {kind} in this namespace."),
            "pointInTime": field("string", "The time to restore fromInstance to. Unset, the latest "
                                 "restorable time.", format="date-time"),
        }, f"Create the {kind_noun(kind)} from another {kind_noun(kind)}'s backups. Set at "
           "creation only.", required=["fromInstance"]),
        "secretName": secret_name("<instance>-dbadmin"),
    }, ["restore"], [
        ("has(self.restore) == has(oldSelf.restore) && (!has(self.restore) || "
         "self.restore == oldSelf.restore)", "restore is set at creation only"),
        ("!has(self.version) || !has(oldSelf.version) || " + same_major + " || "
         "(has(self.allowMajorVersionUpgrade) && self.allowMajorVersionUpgrade)",
         "a major version change needs allowMajorVersionUpgrade"),
        ("!has(self.version) || " + has_minor + " == "
         "(has(self.autoMinorVersionUpgrade) && !self.autoMinorVersionUpgrade)",
         "version takes a minor exactly when autoMinorVersionUpgrade is false" + FROM_CLASS),
    ])


def kind_noun(kind):
    return "cluster" if kind.endswith("Cluster") else "server"


def db_status(engine):
    """A database's status: the common fields as they run, the address and the provider's values."""
    return obj({
        "version": S(),
        "storage": obj({"sizeGB": field("integer", "The actual disk size."),
                        "autoGrow": field("boolean"), "maxSizeGB": field("integer"),
                        "iops": field("integer"), "throughputMBps": field("integer")}),
        "highAvailability": field("boolean"),
        "maintenanceWindow": obj({"day": S(), "hourUTC": field("integer")}),
        "autoMinorVersionUpgrade": field("boolean"),
        "backup": obj({"retentionDays": field("integer"), "pointInTimeDays": field("integer"),
                       "startHourUTC": field("integer")}),
        "endpoint": endpoint(),
        "connectionSecret": field("string", "The admin Secret, <instance>-dbadmin (principle 6)."),
        "providerConfig": obj({"applied": S(), "ignored": array(S())},
                              "The provider configuration entries applied and ignored, as <apiVersion>, "
                              "<kind> (principle 4)."),
        "effective": {"type": "object", "x-kubernetes-preserve-unknown-fields": True,
                      "description": desc("Each effective value and its source: the class's "
                                          "template, the object, or the class's enforced "
                                          "(principle 4).")},
        "provider": {"type": "object", "x-kubernetes-preserve-unknown-fields": True,
                     "description": desc("Values of the applied implementation, in the shape its "
                                         "provider configuration kind's status defines (principle 4).")},
    })


def pg_common():
    return db_common("PostgreSQL", "The major version, such as \"17\". With "
                     "autoMinorVersionUpgrade false, the full version, such as \"17.7\".",
                     "^[0-9]+(\\.[0-9]+)?$")


pg_spec = db_spec("PostgreSQLCluster", "PostgreSQLClass", pg_common(),
                  "self.version.split('.')[0] == oldSelf.version.split('.')[0]",
                  "self.version.contains('.')")
S = lambda: field("string")
pg_status = db_status("PostgreSQL")
write("postgresqlcluster", xrd("PostgreSQLCluster", "postgresqlclusters", "database.entigo.com",
                               pg_spec, pg_status))

pg_class_common = pg_common()
write("postgresqlclass", class_crd("PostgreSQLClass", "postgresqlclasses", "database.entigo.com",
                                   pg_class_common, parameters="PostgreSQLParameters"))
write("postgresqlparameters", parameters_crd("PostgreSQLParameters", "postgresqlparameters",
                                             "database.entigo.com", "PostgreSQL"))

PG_STATUS = {"instanceType": S(), "finalBackup": field(
    "string", "The ID the final backup receives when the cluster is deleted, for fromBackup."),
    "state": S()}
write("postgresqlrdsconfig", provider_config_crd(
    "PostgreSQLRDSConfig", "postgresqlrdsconfigs", "aws.database.entigo.com", "Amazon RDS", {
        "instanceType": field("string", "The RDS instance class, such as db.m7g.large.",
                              default=DEFAULT),
        "parameterGroupName": field("string", "An existing RDS parameter group to use, in place "
                                    "of the object's parameters. A major version change needs a "
                                    "new one, since a group fits one major version."),
        "fromBackup": from_backup("An RDS snapshot ID to create the cluster from, such as "
                                  "status.provider.finalBackup of a deleted cluster."),
    }, {**PG_STATUS, "identifier": S(), "resourceId": S(), "engineVersionActual": S(),
        "latestRestorableTime": field("string", format="date-time")}))
write("postgresqlcloudsqlconfig", provider_config_crd(
    "PostgreSQLCloudSQLConfig", "postgresqlcloudsqlconfigs", "gcp.database.entigo.com",
    "Cloud SQL", {
        "instanceType": field("string", "The Cloud SQL machine tier, such as db-custom-2-7680, "
                              "which belongs to edition.", default=DEFAULT),
        "edition": field("string", "The Cloud SQL edition, such as ENTERPRISE.", default=DEFAULT),
        "diskType": field("string", "The Cloud SQL disk type, such as PD_SSD.", default=DEFAULT),
        "fromBackup": from_backup("A Cloud SQL backup to create the cluster from, as "
                                  "<instance>/<backup run ID>."),
    }, {**PG_STATUS, "id": S(), "connectionName": S(), "maintenanceVersion": S()}))
write("postgresqlociconfig", provider_config_crd(
    "PostgreSQLOCIConfig", "postgresqlociconfigs", "oci.database.entigo.com",
    "OCI Database with PostgreSQL", {
        "instanceType": field("string", "The DB system shape. A .Flex shape needs ocpus and "
                              "memoryGB.", default=DEFAULT),
        "ocpus": field("number", "Required with a .Flex shape.", default=DEFAULT, minimum=1),
        "memoryGB": field("integer", "Required with a .Flex shape.", default=DEFAULT, minimum=1),
        "fromBackup": from_backup("A DB system backup OCID to create the cluster from."),
    }, {**PG_STATUS, "id": S(), "instanceOcpuCount": field("number"),
        "instanceMemorySizeInGbs": field("integer")}))
write("postgresqlflexibleserverconfig", provider_config_crd(
    "PostgreSQLFlexibleServerConfig", "postgresqlflexibleserverconfigs",
    "azure.database.entigo.com", "Azure Database for PostgreSQL flexible server", {
        "instanceType": field("string", "The flexible server SKU, such as GP_Standard_D2ds_v5.",
                              default=DEFAULT),
        "storageTier": field("string", "The disk tier, such as P10, which sets its IOPS.",
                             default=DEFAULT),
    }, {"id": S(), "instanceType": S(), "state": S()}))

# --- MariaDB -------------------------------------------------------------------------------

MARIADB_MAJOR = lambda v: f"({v}.split('.')[0] + '.' + {v}.split('.')[1])"


def mariadb_common():
    return db_common("MariaDB", "The major version, which has two numbers, such as \"11.4\". With "
                     "autoMinorVersionUpgrade false, the full version, such as \"11.4.10\".",
                     "^[0-9]+\\.[0-9]+(\\.[0-9]+)?$")


write("mariadbserver", xrd("MariaDBServer", "mariadbservers", "database.entigo.com",
                           db_spec("MariaDBServer", "MariaDBClass", mariadb_common(),
                                   MARIADB_MAJOR("self.version") + " == "
                                   + MARIADB_MAJOR("oldSelf.version"),
                                   "(self.version.split('.').size() == 3)"),
                           db_status("MariaDB")))
write("mariadbclass", class_crd("MariaDBClass", "mariadbclasses", "database.entigo.com",
                                mariadb_common(), parameters="MariaDBParameters"))
write("mariadbparameters", parameters_crd("MariaDBParameters", "mariadbparameters",
                                          "database.entigo.com", "MariaDB"))
write("mariadbrdsconfig", provider_config_crd(
    "MariaDBRDSConfig", "mariadbrdsconfigs", "aws.database.entigo.com", "Amazon RDS", {
        "instanceType": field("string", "The RDS instance class, such as db.t4g.small.",
                              default=DEFAULT),
        "parameterGroupName": field("string", "An existing RDS parameter group to use, in place "
                                    "of the object's parameters. A major version change needs a "
                                    "new one, since a group fits one major version."),
        "fromBackup": from_backup("An RDS snapshot ID to create the server from, such as "
                                  "status.provider.finalBackup of a deleted server."),
    }, {"instanceType": S(), "finalBackup": field(
        "string", "The ID the final backup receives when the server is deleted, for fromBackup."),
        "state": S(), "identifier": S(), "resourceId": S(), "engineVersionActual": S(),
        "latestRestorableTime": field("string", format="date-time")}))

# --- Valkey ----------------------------------------------------------------------------------


def valkey_common():
    """ValkeyServer's common fields (principle 1): one primary and its read replicas, unsharded."""
    return {
        "version": field("string", "Valkey's major and minor, such as \"8.1\", and the floor "
                         "after: raising it upgrades the server, lowering it is rejected. With "
                         "autoMinorVersionUpgrade, the implementation can move the server past it; "
                         "the status shows the running version.", default=DEFAULT,
                         pattern="^[0-9]+\\.[0-9]+$"),
        "highAvailability": field("boolean", "A read replica in another zone, with automatic "
                                  "failover. Needs readReplicas of at least 1. Refused where the "
                                  "implementation does not support HighAvailability (principle 1).",
                                  default=DEFAULT),
        "readReplicas": field("integer", "Read replicas besides the primary, each a copy of it. "
                              "Refused where the implementation does not support ReadReplicas "
                              "(principle 1).", default=DEFAULT, minimum=0),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenanceWindow": window(),
        "autoMinorVersionUpgrade": field("boolean", "Allow the implementation to upgrade to newer "
                                         "minor versions on its schedule. false keeps the server "
                                         "at version.", default=DEFAULT),
        "backup": obj({
            "retentionDays": field("integer", "Days automated backups are kept, within the "
                                   "implementation's own range (principle 10). 0 turns automated "
                                   "backups off.", default=DEFAULT, minimum=0),
            "startHourUTC": field("integer", "The hour, in UTC, the daily backup starts.",
                                  minimum=0, maximum=23),
        }, "Automated daily backups."),
        "parametersRefs": parameters_refs("ValkeyParameters", "Reusable Valkey settings"),
        "parameters": string_map("Valkey settings for this object only, merged after "
                                 "parametersRefs."),
    }


write("valkeyserver", xrd("ValkeyServer", "valkeyservers", "database.entigo.com",
                          class_spec("ValkeyClass", valkey_common(), {
                              "secretName": secret_name("<instance>-credentials")}, [], [
                              ("!has(self.highAvailability) || !self.highAvailability || "
                               "(has(self.readReplicas) && self.readReplicas >= 1)",
                               "highAvailability needs readReplicas of at least 1" + FROM_CLASS),
                          ]),
                          obj({
                              "version": S(),
                              "highAvailability": field("boolean"),
                              "readReplicas": field("integer"),
                              "maintenanceWindow": obj({"day": S(), "hourUTC": field("integer")}),
                              "autoMinorVersionUpgrade": field("boolean"),
                              "backup": obj({"retentionDays": field("integer"),
                                             "startHourUTC": field("integer")}),
                              "endpoint": endpoint("The primary's address."),
                              "readerEndpoint": endpoint("The address that spreads reads over the "
                                                         "replicas."),
                              "connectionSecret": field("string", "The Secret "
                                                        "<instance>-credentials (principle 6)."),
                              "providerConfig": obj({"applied": S(), "ignored": array(S())},
                                                    "The provider configuration entries applied and "
                                                    "ignored, as <apiVersion>, <kind> (principle 4)."),
                              "effective": {"type": "object",
                                            "x-kubernetes-preserve-unknown-fields": True,
                                            "description": desc("Each effective value and its "
                                                                "source: the class's template, the "
                                                                "object, or the class's enforced "
                                                                "(principle 4).")},
                              "provider": {"type": "object",
                                           "x-kubernetes-preserve-unknown-fields": True,
                                           "description": desc("Values of the applied "
                                                               "implementation, in the shape its "
                                                               "provider configuration kind's status "
                                                               "defines (principle 4).")},
                          })))
write("valkeyclass", class_crd("ValkeyClass", "valkeyclasses", "database.entigo.com",
                               valkey_common(), parameters="ValkeyParameters"))
write("valkeyparameters", parameters_crd("ValkeyParameters", "valkeyparameters",
                                         "database.entigo.com", "Valkey"))
write("valkeyelasticacheconfig", provider_config_crd(
    "ValkeyElastiCacheConfig", "valkeyelasticacheconfigs", "aws.database.entigo.com",
    "ElastiCache", {
        "instanceType": field("string", "The ElastiCache node type, such as cache.t4g.small.",
                              default=DEFAULT),
        "parameterGroupName": field("string", "An existing ElastiCache parameter group to use, in "
                                    "place of the object's parameters. A major version change "
                                    "needs a new one, since a group fits one major version."),
        "fromBackup": from_backup("An ElastiCache snapshot name to create the server from, such "
                                  "as status.provider.finalBackup of a deleted server."),
    }, {"instanceType": S(), "finalBackup": field(
        "string", "The ID the final snapshot receives when the server is deleted, for fromBackup."),
        "engineVersionActual": S()}))
write("valkeymemorystoreconfig", provider_config_crd(
    "ValkeyMemorystoreConfig", "valkeymemorystoreconfigs", "gcp.database.entigo.com",
    "Memorystore for Valkey", {
        "instanceType": field("string", "The Memorystore node type, such as HIGHMEM_MEDIUM.",
                              default=DEFAULT),
    }, {"id": S(), "instanceType": S()}))
write("valkeyocicacheconfig", provider_config_crd(
    "ValkeyOCICacheConfig", "valkeyocicacheconfigs", "oci.database.entigo.com", "OCI Cache", {
        "memoryGB": field("integer", "Memory per node, within OCI's own range (principle 10).",
                          default=DEFAULT, minimum=1),
    }, {"id": S(), "memoryGB": field("integer"), "state": S(), "softwareVersion": S()}))

# --- Kafka -----------------------------------------------------------------------------------


def kafka_common():
    """KafkaCluster's common fields (principle 1)."""
    return {
        "version": field("string", "The Kafka version, such as \"4.0\". Each implementation "
                         "accepts the versions it offers (principle 10).", default=DEFAULT),
        "brokers": field("integer", "The number of brokers.", default=DEFAULT, minimum=1),
        "storage": obj({"sizeGB": field("integer", "Disk per broker, in GB.", default=DEFAULT,
                                        minimum=1)}),
        "highAvailability": field("boolean", "Survive the loss of one zone. Refused where the "
                                  "implementation does not support HighAvailability (principle 1).",
                                  default=DEFAULT),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "parametersRefs": parameters_refs("KafkaParameters", "Reusable broker settings"),
        "parameters": string_map("Broker settings for this object only, merged after "
                                 "parametersRefs."),
    }


write("kafkacluster", xrd("KafkaCluster", "kafkaclusters", "queue.entigo.com",
                          class_spec("KafkaClass", kafka_common(), {}, [], []),
                          obj({
                              "version": S(),
                              "brokers": field("integer"),
                              "storage": obj({"sizeGB": field("integer")}),
                              "highAvailability": field("boolean"),
                              "bootstrapServers": field("string", "The listener applications "
                                                        "connect to, as host:port pairs."),
                              "providerConfig": obj({"applied": S(), "ignored": array(S())},
                                                    "The provider configuration entries applied and "
                                                    "ignored, as <apiVersion>, <kind> (principle 4)."),
                              "effective": {"type": "object",
                                            "x-kubernetes-preserve-unknown-fields": True,
                                            "description": desc("Each effective value and its "
                                                                "source: the class's template, the "
                                                                "object, or the class's enforced "
                                                                "(principle 4).")},
                              "provider": {"type": "object",
                                           "x-kubernetes-preserve-unknown-fields": True,
                                           "description": desc("Values of the applied "
                                                               "implementation, in the shape its "
                                                               "provider configuration kind's status "
                                                               "defines (principle 4).")},
                          })))
write("kafkaclass", class_crd("KafkaClass", "kafkaclasses", "queue.entigo.com", kafka_common(),
                              parameters="KafkaParameters", extra={"topicTemplate": obj({
                                  "partitions": field("integer", minimum=1),
                                  "replicationFactor": field("integer", minimum=1),
                              }, "Defaults written into a KafkaTopic on a cluster of this class "
                                 "when the topic is created; the topic may change them.")}))
write("kafkaparameters", parameters_crd("KafkaParameters", "kafkaparameters", "queue.entigo.com",
                                        "Broker"))
write("kafkamskconfig", provider_config_crd(
    "KafkaMSKConfig", "kafkamskconfigs", "aws.queue.entigo.com", "Amazon MSK", {
        "instanceType": field("string", "The MSK broker instance type, such as kafka.m7g.large.",
                              default=DEFAULT),
    }, {"instanceType": S(), "arn": S(), "kafkaVersion": S()}))
write("kafkagcpconfig", provider_config_crd(
    "KafkaGCPConfig", "kafkagcpconfigs", "gcp.queue.entigo.com",
    "Google Cloud Managed Service for Apache Kafka", {
        "vcpus": field("integer", "The whole cluster's vCPUs.", default=DEFAULT, minimum=1),
        "memoryGB": field("integer", "The whole cluster's memory, within GCP's range per vCPU "
                          "(principle 10).", default=DEFAULT, minimum=1),
    }, {"id": S(), "vcpus": field("integer"), "memoryGB": field("integer")}))
write("kafkaociconfig", provider_config_crd(
    "KafkaOCIConfig", "kafkaociconfigs", "oci.queue.entigo.com",
    "OCI Streaming with Apache Kafka", {
        "instanceType": field("string", "The broker shape. A .Flex shape needs ocpus.",
                              default=DEFAULT),
        "ocpus": field("number", "OCPUs per broker. Required with a .Flex shape.",
                       default=DEFAULT, minimum=1),
    }, {"id": S(), "instanceType": S(), "ocpus": field("number"), "state": S()}))

# --- RabbitMQ --------------------------------------------------------------------------------


def rabbitmq_common():
    """RabbitMQBroker's common fields (principle 1)."""
    return {
        "version": field("string", "RabbitMQ's release line, such as \"4.2\". The implementation "
                         "manages the patch.", default=DEFAULT, pattern="^[0-9]+\\.[0-9]+$"),
        "highAvailability": field("boolean", "A three-node cluster across zones. Refused where the "
                                  "implementation does not support HighAvailability (principle 1).",
                                  default=DEFAULT),
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "maintenanceWindow": window(),
        "parametersRefs": parameters_refs("RabbitMQParameters", "Reusable rabbitmq.conf settings"),
        "parameters": string_map("rabbitmq.conf settings for this object only, merged after "
                                 "parametersRefs."),
    }


write("rabbitmqbroker", xrd("RabbitMQBroker", "rabbitmqbrokers", "mq.entigo.com",
                            class_spec("RabbitMQClass", rabbitmq_common(), {
                                "secretName": secret_name("<broker>-credentials")}, [], []),
                            obj({
                                "version": S(),
                                "highAvailability": field("boolean"),
                                "maintenanceWindow": obj({"day": S(), "hourUTC": field("integer")}),
                                "endpoint": endpoint("The AMQPS address."),
                                "managementEndpoint": field("string", "The management console URL."),
                                "connectionSecret": field("string", "The Secret "
                                                          "<broker>-credentials (principle 6)."),
                                "providerConfig": obj({"applied": S(), "ignored": array(S())},
                                                      "The provider configuration entries applied "
                                                      "and ignored, as <apiVersion>, <kind> "
                                                      "(principle 4)."),
                                "effective": {"type": "object",
                                              "x-kubernetes-preserve-unknown-fields": True,
                                              "description": desc("Each effective value and its "
                                                                  "source: the class's template, "
                                                                  "the object, or the class's "
                                                                  "enforced (principle 4).")},
                                "provider": {"type": "object",
                                             "x-kubernetes-preserve-unknown-fields": True,
                                             "description": desc("Values of the applied "
                                                                 "implementation, in the shape its "
                                                                 "provider configuration kind's "
                                                                 "status defines (principle 4).")},
                            })))
write("rabbitmqclass", class_crd("RabbitMQClass", "rabbitmqclasses", "mq.entigo.com",
                                 rabbitmq_common(), parameters="RabbitMQParameters"))
write("rabbitmqparameters", parameters_crd("RabbitMQParameters", "rabbitmqparameters",
                                           "mq.entigo.com", "rabbitmq.conf"))
write("rabbitmqamazonmqconfig", provider_config_crd(
    "RabbitMQAmazonMQConfig", "rabbitmqamazonmqconfigs", "aws.mq.entigo.com", "Amazon MQ", {
        "instanceType": field("string", "The Amazon MQ host instance type, such as "
                              "mq.m7g.medium.", default=DEFAULT),
        "publiclyAccessible": field("boolean", "Reachable from the internet, without the "
                                    "platform's security group. Amazon MQ replaces the broker to "
                                    "change it, so it is set at creation only.",
                                    default="fixed, false"),
    }, {"instanceType": S(), "id": S(), "engineVersion": S()}))

# --- Bucket ----------------------------------------------------------------------------------


def bucket_common():
    """Bucket's common fields (principle 1)."""
    return {
        "versioning": field("boolean", "Keep replaced and deleted objects as versions.",
                            default=DEFAULT),
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
                "tier": field("string", "One of the two cold tiers every implementation has.",
                              enum=["Infrequent", "Archive"]),
            }, required=["afterDays", "tier"]), "Move objects to a colder tier."),
        })),
        "retention": obj({
            "days": field("integer", minimum=1),
            "locked": field("boolean", "true cannot be undone in any implementation.",
                            **{"x-kubernetes-validations": [{
                                "rule": "!oldSelf || self",
                                "message": "a locked retention cannot be unlocked"}]}),
        }, "Objects cannot be changed or deleted for this many days.", required=["days"]),
        "cors": array(obj({
            "origins": array(field("string")),
            "methods": array(field("string", enum=["GET", "PUT", "POST", "DELETE", "HEAD"])),
            "maxAgeSeconds": field("integer", minimum=0),
        }, required=["origins", "methods"]), "Cross-origin requests browsers may make."),
        "softDeleteDays": field("integer", "Days deleted objects can be recovered.",
                                default=DEFAULT, minimum=0),
    }


write("bucket", xrd("Bucket", "buckets", "storage.entigo.com",
                    class_spec("BucketClass", bucket_common(), {}, [], [
                        ("!has(oldSelf.retention) || !has(oldSelf.retention.locked) || "
                         "!oldSelf.retention.locked || (has(self.retention) && "
                         "has(self.retention.locked) && self.retention.locked)",
                         "a locked retention cannot be unlocked or removed"),
                    ]),
                    obj({
                        "url": S(),
                        "bucketName": S(),
                        "region": S(),
                        "providerConfig": obj({"applied": S(), "ignored": array(S())},
                                              "The provider configuration entries applied and "
                                              "ignored, as <apiVersion>, <kind> (principle 4)."),
                        "effective": {"type": "object",
                                      "x-kubernetes-preserve-unknown-fields": True,
                                      "description": desc("Each effective value and its source: "
                                                          "the class's template, the object, or "
                                                          "the class's enforced (principle 4).")},
                        "provider": {"type": "object",
                                     "x-kubernetes-preserve-unknown-fields": True,
                                     "description": desc("Values of the applied implementation, "
                                                         "in the shape its provider configuration "
                                                         "kind's status defines (principle 4).")},
                    })))
write("bucketclass", class_crd("BucketClass", "bucketclasses", "storage.entigo.com",
                               bucket_common()))
write("buckets3config", provider_config_crd(
    "BucketS3Config", "buckets3configs", "aws.storage.entigo.com", "Amazon S3", {},
    {"arn": S()}))
write("bucketcloudstorageconfig", provider_config_crd(
    "BucketCloudStorageConfig", "bucketcloudstorageconfigs", "gcp.storage.entigo.com",
    "Cloud Storage", {}, {"selfLink": S()}))
write("bucketobjectstorageconfig", provider_config_crd(
    "BucketObjectStorageConfig", "bucketobjectstorageconfigs", "oci.storage.entigo.com",
    "OCI Object Storage", {}, {"namespace": S(), "retentionLockedFrom": field(
        "string", "The time a locked retention takes effect.", format="date-time")}))
write("bucketblobstorageconfig", provider_config_crd(
    "BucketBlobStorageConfig", "bucketblobstorageconfigs", "azure.storage.entigo.com",
    "Azure Blob Storage", {}, {"accountId": S(), "containerId": S()}))

# --- ArtifactRepository ----------------------------------------------------------------------


def artifact_repository_common():
    """ArtifactRepository's common fields (principle 1)."""
    return {
        "deletionProtection": field("boolean", "Deleting the object is denied while true.",
                                    default="fixed, true"),
        "immutableTags": field("boolean", "A pushed tag cannot be pushed again.", default=DEFAULT),
        "lifecycleRules": array(obj({
            "tagPrefixes": array(field("string"), "Images with a tag starting with one of "
                                 "these."),
            "tagPatterns": array(field("string"), "Images with a tag matching one of these "
                                 "wildcards."),
            "untagged": field("boolean", "Images without a tag."),
            "keepCount": field("integer", "Keep this many newest images.", minimum=1),
            "expireAfterDays": field("integer", "Delete images this long after their push.",
                                     minimum=1),
        }, "One selector, or none for every image, and one bound.", rules=[
            ("(has(self.tagPrefixes) ? 1 : 0) + (has(self.tagPatterns) ? 1 : 0) + "
             "(has(self.untagged) ? 1 : 0) <= 1", "at most one selector"),
            ("has(self.keepCount) != has(self.expireAfterDays)",
             "exactly one of keepCount and expireAfterDays"),
        ]), "Delete old images. Set, it replaces the class's rules.", default=DEFAULT),
    }


write("artifactrepository", xrd(
    "ArtifactRepository", "artifactrepositories", "artifact.entigo.com",
    class_spec("ArtifactRepositoryClass", artifact_repository_common(), {
        "name": field("string", "The image name. Unset, the object's name. Set at creation "
                      "only."),
        "path": field("string", "A prefix between the namespace and the name: the image path "
                      "is <namespace>/<path>/<name>, or <namespace>/<name> unset. Set at "
                      "creation only."),
    }, [], [
        ("(has(self.name) ? self.name : '') == (has(oldSelf.name) ? oldSelf.name : '')",
         "name is immutable"),
        ("(has(self.path) ? self.path : '') == (has(oldSelf.path) ? oldSelf.path : '')",
         "path is immutable"),
    ]),
    obj({
        "uri": field("string", "The image URI, such as "
                     "123456789012.dkr.ecr.eu-north-1.amazonaws.com/shop/orders-api."),
        "providerConfig": obj({"applied": S(), "ignored": array(S())},
                              "The provider configuration entries applied and ignored, as "
                              "<apiVersion>, <kind> (principle 4)."),
        "effective": {"type": "object", "x-kubernetes-preserve-unknown-fields": True,
                      "description": desc("Each effective value and its source: the class's "
                                          "template, the object, or the class's enforced "
                                          "(principle 4).")},
        "provider": {"type": "object", "x-kubernetes-preserve-unknown-fields": True,
                     "description": desc("Values of the applied implementation, in the shape its "
                                         "provider configuration kind's status defines "
                                         "(principle 4).")},
    })))
write("artifactrepositoryclass", class_crd("ArtifactRepositoryClass", "artifactrepositoryclasses",
                                           "artifact.entigo.com", artifact_repository_common()))
write("artifactrepositoryecrconfig", provider_config_crd(
    "ArtifactRepositoryECRConfig", "artifactrepositoryecrconfigs", "aws.artifact.entigo.com",
    "Amazon ECR", {}, {"arn": S()}))
write("artifactrepositoryartifactregistryconfig", provider_config_crd(
    "ArtifactRepositoryArtifactRegistryConfig", "artifactrepositoryartifactregistryconfigs",
    "gcp.artifact.entigo.com", "Artifact Registry", {}, {"id": S()}))
write("artifactrepositoryocirconfig", provider_config_crd(
    "ArtifactRepositoryOCIRConfig", "artifactrepositoryocirconfigs", "oci.artifact.entigo.com",
    "OCIR", {}, {"id": S()}))
write("artifactrepositoryacrconfig", provider_config_crd(
    "ArtifactRepositoryACRConfig", "artifactrepositoryacrconfigs", "azure.artifact.entigo.com",
    "Azure Container Registry", {}, {"registryId": S()}))
