"""Check that every example manifest in the design documents passes its draft schema.

Usage: python3 test_examples.py <schemas directory> <kubeconfig>

Installs the schemas as CRDs, as test_rules.py does, then sends each v1alpha2 manifest found in a
yaml block of the documents next to the schemas directory to the API server with a server-side dry
run. Manifests of today's APIs, in other versions, are skipped.
"""
import json
import re
import subprocess
import sys
from pathlib import Path

import yaml

SCHEMAS = Path(sys.argv[1])
KUBECONFIG = sys.argv[2]
DOCS = SCHEMAS.resolve().parent


def kubectl(*args, stdin=None):
    r = subprocess.run(["kubectl", "--kubeconfig", KUBECONFIG, *args], input=stdin,
                       capture_output=True, text=True)
    return r.returncode, (r.stdout + r.stderr).strip()


def install():
    versions = set()
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
        if code:
            sys.exit(f"INSTALL FAILED {f.name}\n{out}")
        versions.add(s["group"] + "/" + v["name"])
    kubectl("wait", "--for=condition=Established", "crd", "--all", "--timeout=60s")
    return versions


def examples():
    for f in sorted(DOCS.glob("*.md")):
        for block in re.findall(r"^```yaml\n(.*?)^```", f.read_text(), re.M | re.S):
            for doc in yaml.safe_load_all(block):
                if isinstance(doc, dict) and "kind" in doc and "apiVersion" in doc:
                    yield f.name, doc


served = install()
results = []
for name, doc in examples():
    if doc["apiVersion"] not in served:
        continue
    ns = doc.get("metadata", {}).get("namespace")
    if ns:
        kubectl("create", "namespace", ns)
    code, out = kubectl("apply", "--dry-run=server", "-f", "-", stdin=json.dumps(doc))
    label = f"{name}: {doc['kind']} {doc.get('metadata', {}).get('name', '')}"
    results.append(code == 0)
    print(("PASS " if code == 0 else "FAIL ") + label + ("" if code == 0 else "\n     " + out[-600:]))
print(f"\n{sum(results)} of {len(results)} passed")
sys.exit(0 if all(results) else 1)
