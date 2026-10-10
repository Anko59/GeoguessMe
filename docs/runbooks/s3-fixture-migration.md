---
status: primary
---

# Local S3 fixture migration

## Scope and safety

The local/disposable fixture now uses official SeaweedFS 4.48, pinned by digest
and release signature. Hosted Cloudflare R2 is **unchanged**. The Compose
service and DNS alias remain `minio`, S3 remains port 9000, and the optional
production fixture profile remains `local-minio`. These are compatibility names,
not MinIO server software. Migration clients require Linux host networking;
Docker Desktop requires its host-networking feature enabled. Unsupported routes
fail closed; never expose the reader/staging listener publicly as a workaround.
No port-9001 console or unauthenticated replacement UI is published. Fixture
credentials are deliberately local-only `minioadmin` / `minioadmin`; never
expose this fixture publicly or reuse them for hosted data.

SeaweedFS cannot read MinIO's on-disk format. The new development volume is
`geoguessme-dev_geoguessme_dev_s3_fixture`; the legacy volume
`geoguessme-dev_geoguessme_dev_minio` remains unmanaged and is never reset,
removed, or mounted into SeaweedFS. Normal development startup refuses legacy
data without a verified migration receipt. Ordinary `make down` preserves both.
Even new Compose volume cleanup cannot remove the unmanaged legacy source, but
it can delete the new target; the resulting receipt then fails closed. The
lightweight default is 32 volumes of 64 MiB (about 2 GiB raw capacity). For a
larger import, review free disk space and export the positive integer
`GEOGUESSME_S3_FIXTURE_VOLUME_MAX` (maximum 4096), keeping the same value
through staging and normal development. Exhaustion aborts copying without an
activation receipt; it never justifies deleting source data. Tooling values are
shown in the
[fixture environment example](../../deployment/env/s3-fixture.env.example).

Migration copies **current objects** in the default application's
`geoguessme-media` bucket, preserves content bytes plus HTTP/user metadata, and
independently verifies sorted keys, counts, sizes and SHA-256 hashes. Existing
identical target objects are revalidated and skipped; conflicting objects are
not overwritten. Extra target keys invalidate verification. Historical versions,
delete markers, tags, ACLs, policies, other buckets and original timestamps are
**not migrated**. Preserve the source/snapshot and separately review those
features or custom KMS configuration before any manual retirement decision.

## Already-running legacy source

First [back up the development database](../operations.md#database) and preserve
the old storage volume. The migration is explicit, quiesces backend/frontend
writers, and never launches an archived source server as an implicit fallback:

```text
make bootstrap
make dev-s3-migrate
make dev-s3-guard
make dev
```

The source must already be the single running `geoguessme-dev` MinIO container,
using the actual legacy volume at `/data` and local port 9000. Isolate its old
listener from untrusted networks while copying. If old fixture credentials were
customized, supply `SOURCE_S3_FIXTURE_ACCESS_KEY` and
`SOURCE_S3_FIXTURE_SECRET_KEY` through the operator environment; do not print
secrets or put them in chat. Source and target endpoints are forced to loopback,
so hosted R2 cannot be selected by inherited application configuration.

Staging exposes only `127.0.0.1:19000` and mounts the **same new volume** used
by normal development. After copy and complete read-only verification, staging
must visibly stop. A private receipt under `.local/s3-fixture-migration` binds
both actual volume names/creation identities, bucket, object count and manifest
hash. Recreated volumes invalidate it. A fail-closed owned mutex prevents
concurrent migrations/recovery. The guard does not continuously compare object
contents after normal application writes; it records the original verified
transition, not a permanent immutability claim.

## Offline legacy source recovery

An offline source fails closed. For this default local fixture only, the
following **explicitly confirmed** recovery first creates a private raw offline
snapshot from a read-only original-volume mount, verifies its checksum, and then
starts a temporary archived reader:

```text
make bootstrap
make dev-s3-recovery-source CONFIRM=legacy-s3-recovery
make dev-s3-migrate
make dev-s3-guard
make dev
```

This reader is the exact retired immutable MinIO image, **not maintained or
approved as a new runtime fixture**. It is an isolated transition tool: internal
network, loopback-only port 9000, no public console, no Docker socket, dropped
capabilities and no privilege escalation. Its original image runs as root to
read the legacy root-owned files. Root filesystem is read-only with temporary
HOME; the legacy data mount is writable because MinIO startup can update
internal metadata. The completed private raw snapshot preserves the pre-start
files and ownership before that happens. Do not use this recovery for hosted
storage or launch it without the snapshot/confirmation checks.

Migration stops only its specially labelled managed reader after successful
verification and observes shutdown before issuing a receipt. To abort recovery:

```text
make dev-s3-recovery-source-stop
make dev-s3-stage-stop
```

These commands never delete either data volume or the completed snapshot. A
foreign container, occupied port, running source-volume user, failed snapshot,
failed verification, or failed shutdown aborts without activation proof.

## Failure and rollback

Do not run reset/prune commands to repair a failed migration. Keep the original
volume, private snapshot and database backup. A partial target can be retried:
identical objects are verified, while conflicts require explicit investigation.
An interrupted mutex requires confirming no owner/reader/staging process remains
before deliberate operator cleanup; never delete another active process's lock.

To roll back after activation, stop application writers and the new fixture,
preserve all new writes and the database state, then use the reviewed legacy
source/snapshot with the matching old application configuration. **Do not**
mount SeaweedFS data into MinIO, silently merge diverged stores, or
automatically restore database contents. Raw snapshot restoration is a
separately reviewed manual recovery action, not performed by these targets.

## Validation

```text
make test-s3-fixture
make test-s3-fixture-race
make test-s3-fixture-integration
make verify-s3-upstream
make restart-rehearsal
```

The real-image regression uses a uniquely allocated disposable project, verifies
signed authentication and anonymous rejection, conditional no-overwrite,
copy/manifest integrity, and persistence across restart. It never reads or
migrates the user's development volume. Full release gate requirements remain in
the [testing guide](../testing.md) and
[security scanning guide](../security-scanning.md).
