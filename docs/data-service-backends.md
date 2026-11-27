# Data Service Backends

A data service kind of the Portable Data Service APIs, such as `PostgreSQLCluster`, runs on a
backend: one cloud service, composed by a backend package in the kind's function. The kind's own
package holds what every backend shares (the entry that applies, the effective spec, the status
and the admission rules), and each backend composes its own resources.
`functions/database/service/pgcluster/` is the worked example: `cluster.go` and `backend.go` are
the shared part, `oci/` the OCI Database with PostgreSQL backend.

This document holds the rules a backend has to follow and the provider behaviour it has to be
tested against. Each rule names the failure it prevents.

## Function Invariants

The function runs on every reconcile and is the only place that sees the composite, its required
resources and its observed composed resources together. A property that binds a cluster to a
cloud resource is held there, on every reconcile:

- **Resource identity.** The composed resource names the cloud resource the cluster was created with
  or observed, and an entry whose `id` differs is refused, in either direction. Admission sees only
  the object being edited: an entry switched to a `ref`, or a referenced object deleted and created
  again, passes it. The OCI backend records the observed `id` in the annotation
  `database.entigo.com/existing-id` and compares it (`oci/oci.go`, `d5e730a`).
- **Creation-time values.** A value that applies at creation only, such as the backup a cluster is
  restored from, is taken from the composed resource once it exists, not from the entry.
- **Placement.** A subnet, an availability domain or a compartment is taken from the existing
  resource, not recomputed from the deployment's settings, which may change after the resource was
  created; a provider refuses the move or replaces the resource.
- **Connection Secret ownership.** A `secretName` that names a Secret another owner controls is
  refused (`cluster.go`, `checkSecret`). Without the check, Crossplane takes the Secret over, writes
  the connection details into it and deletes it with the cluster.

## Composed Resources

Crossplane v2 deletes a composed resource the function stops returning, and a fatal function result
changes nothing. A backend therefore keeps returning a resource whose inputs are briefly missing,
such as a security group whose status has no OCID yet or a database whose endpoint is not
reported, and fails the reconcile when it cannot compose safely.

A composed managed resource takes the management policies `Observe`, `Create`, `Update` and
`Delete`, without `LateInitialize`. With `LateInitialize`, the provider copies its state into the
spec and owns fields such as a list of patch operations, which the function then cannot remove.
Without it, the provider never writes the external name, so the function writes the observed ID to
`crossplane.io/external-name`; otherwise a restarted provider loses the resource, creates another
one, and leaves the first one in the cloud when the cluster is deleted.

## Provider Behaviour

An upjet provider has behaviour its CRDs do not show. Before designing a composition, test each
field the backend sets by hand against these cases:

| Behaviour                                                  | Effect                                                                                                                    | Handling                                              |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Field never read back after creation                       | Every later update plans a replacement and is refused                                                                     | Set the field in `initProvider` only                  |
| Empty list dropped                                         | A list emptied on purpose keeps its previous value                                                                        | State the empty case explicitly, such as `kind: NONE` |
| Optional and computed field left out                       | The provider keeps the cloud's value                                                                                      | Always state the value the cluster wants              |
| Change that forces replacement                             | upjet refuses the update with `refuse to update the external resource because the following update requires replacing it` | Refuse the change in admission or the function first  |
| Write-only field on a resource the provider did not create | The state built from the cloud lacks the field, so setting it reads as a change                                           | Do not send the field for an adopted resource         |
| Asynchronous create                                        | A provider restart during the create makes a second resource                                                              | Keep provider pods from voluntary eviction            |

*Upjet provider behaviour to test for each composed field.*

The OCI backend's instances of each, with the provider versions, are in
`compositions/postgresqlcluster/README.md`.

## Existing Resources

Observing an existing resource and taking it over follow the common design's observed-resource
rules. A backend needs, beyond `managementPolicies: [Observe]`:

1. A workspace on the function (`WORKSPACE`), since the tag check compares the resource's workspace
   tag with it; a deployment without one refuses `observe`.
2. A check of the workspace and Zone tags on every reconcile while observing, and of the Zone tag at
   takeover (`pgcluster.CheckTags`).
3. Values taken from the existing resource for each field the cluster leaves unset at takeover,
   recorded with the source `cloud`.
4. The owner's own tags and security groups merged with the platform's, not replaced.
5. A connection Secret without a password while observing: no cloud returns an admin password.

An observed resource's details appear in the managed resource's status before the tag check can
run, so anyone who can read managed resources in the namespace reads them; the check cannot come
first.

## Status

The status holds the common fields as the backend runs them, `status.effective` with each value
and its source (`object`, `object/<Kind>` for an entry's setting, `implementation`, `cloud`), and
`status.provider` with the backend's own values. The backend reports its entry's settings through
`Backend.Effective`.

A value the cluster no longer has is removed by giving it a nil value in the status the function
returns (`functions/common/base/fn.go`, `addStatus`); a value kept through a gap in what the
provider reports, such as an endpoint, is returned unchanged.

The condition `CloudAccepted` is reported on every reconcile, since Crossplane keeps a condition a
function stops returning. It is false while a composed managed resource's create, update, delete or
observe fails, with that resource's message, and for a refusal the function marks with
`base.NotAccepted`, such as an operation the backend cannot perform. Crossplane applies the
conditions of a fatal result too, so a refusal stays fatal and changes nothing composed.

## Admission

Rules that admission can hold go in the kind's `ValidatingAdmissionPolicy`: required fields,
creation-only fields of an inline entry (written with `oldObject`), and fields outside the current
iteration. A rule on a referenced provider configuration object holds only within that object,
and the function holds the rest (see Function Invariants). Each rule is tested against a real API
server, since a rule that compiles may still not match.

## Testing

A backend is tested at four levels:

1. Unit tests of the function, one for each rule above, written to fail without the change they
   guard.
2. The composition's render test (`compositions/<kind>/test`), and the render tests of every other
   kind the same function serves, since the function's shared code changes with the backend. The
   render tests start their function on port 9443, so they run one package at a time
   (`go test -p 1`).
3. A live test on a deployment of the backend's cloud: create, change each field, observe, take
   over, delete. A fresh managed resource is not enough; most provider behaviour above appears only
   on the second update or after a restart.
4. A cold review of the whole branch by a reviewer without the author's context, repeated after
   each round of fixes until a round finds nothing that needs a fix.
