# Release Baseline

This release manager reference defines the release scope for Matilda Discovery Readiness Toolkit. Operators should start with the root [README](../../README.md) or [Quickstart](../user/quickstart.md).

## Supported Today

- Linux target readiness for targets reached directly from the operator machine.
- Linux target readiness for targets reached through MatildaProbeVM.
- Default `inventory.yml` and normalized v1 inventory for Linux execution.
- Local terminal console and local browser UI.
- Local run history under `.matilda/`.
- Readiness reports under `reports/`.

## Validated Workflows

The release baseline validates these Linux workflows:

- `./matilda-prep doctor`
- `./matilda-prep inventory validate`
- `./matilda-prep preflight`
- `./matilda-prep setup`
- `./matilda-prep validate`
- `./matilda-prep report`
- `./matilda-prep rollback --sudoers-only`
- `./matilda-prep rollback --remove-key`
- `./matilda-prep rollback --lock-user`
- `./matilda-prep rollback --delete-user`
- `./matilda-prep ui`

`setup` and rollback modes modify Linux targets and require explicit confirmation.

## Guidance Only

These commands generate local guidance artifacts only. They do not connect to targets and do not change target configuration.

- `./matilda-prep generate windows`
- `./matilda-prep generate unix`

Windows remote automation and UNIX remote automation are not implemented in this release.

## Not Automated

These areas are not automated in this release:

- Cloud API readiness for AWS, Azure, GCP, and OCI.
- Kubernetes API readiness.
- Windows remote setup and validation.
- UNIX remote setup and validation.

## Prerequisites

Operator machine:

- Linux or macOS with Bash.
- Go when cloning and running from source.
- Ansible.
- SSH access to the target admin account.
- SSH access to MatildaProbeVM when private targets or Probe validation are used.
- Local `.env` and `inventory.yml`.

Windows operator machines are not validated in this release. For WSL source checkouts, configure Go, Ansible, and SSH. For WSL release packages, use the Linux package and configure Ansible and SSH.

Linux targets:

- SSH reachable by the operator machine directly or through MatildaProbeVM.
- Admin account with non-interactive sudo for setup.
- Probe-to-target TCP/22 reachability.
- Oracle Linux / RHEL-like systems are the validated baseline.

MatildaProbeVM:

- SSH reachable by the operator machine when private targets are used.
- Has the Matilda discovery private key at the path configured in `.env`.
- Has `nc` or `ncat` for Probe-to-target TCP checks.

## Limitations

- Matilda discovery is agentless and read-only, but toolkit `setup` changes Linux targets by creating or updating `matilda-svc`, installing a public key, and writing sudoers configuration.
- Private keys must not be copied to targets.
- Reports are local generated artifacts and are ignored by git.
- `.matilda/` run history is local state and is ignored by git.
- Non-Linux v1 inventory targets are structurally valid, but Linux remote actions skip them.
- Packaged release tarballs include the project files and a prebuilt `matilda-prep` binary for one operating system and CPU architecture.
- Standalone binary assets are not one-file installs. They must be run from a source checkout or extracted package root so the toolkit can find its Ansible, template, schema, and documentation files.
- A source clone can be used on validated operator platforms with Go installed.

## Branch And Tag Workflow

Release work follows the protected-main workflow in [Branching Workflow](BRANCHING.md).

1. Start from current `dev`.
2. Create a short-lived release or packaging branch.
3. Read related implementation, docs, packaging, and validation files before editing.
4. Run local validation and review the diff before opening a pull request.
5. Merge the release work into `dev` by pull request only after review and CI pass.
6. Promote `dev` into `main` by a separate pull request only after release validation and CI pass.
7. Create release tags from `main`.
8. Do not sync `main` back into `dev` after release unless there is a real file difference, conflict, or branch-protection blocker.
9. After a release is published, prefer a new tag over moving the published tag unless the tag move is explicitly approved. For RCs, use the next RC tag.

## Release Validation

Before tagging a release or RC, run:

```bash
GOCACHE=/private/tmp/matilda-gocache go test ./...
GOCACHE=/private/tmp/matilda-gocache go vet ./...
GOCACHE=/private/tmp/matilda-gocache go build -o /private/tmp/matilda-prep-check ./cmd/matilda-prep
git diff --check
bash -n matilda-prep
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/preflight.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/setup.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/validate.yml
ANSIBLE_CONFIG=ansible/ansible.cfg ANSIBLE_LOCAL_TEMP=/private/tmp/matilda-ansible-local ansible-playbook --syntax-check ansible/playbooks/linux/rollback.yml
```

Then run the [operator smoke test](../user/operator-smoke-test.md) against a fresh clone from origin.

## Release Asset Packaging

Package release tarballs from a clean staging directory built from `git archive`.
Do not tar the working tree directly from macOS, because Finder and copyfile
metadata can add AppleDouble files or extended attributes that show up during
Linux extraction.

For each target operating system and architecture:

1. Build the matching `matilda-prep` binary.
2. Create a clean stage from the tagged checkout with `git archive`.
3. Copy the binary to the staged project root as `matilda-prep`.
4. Strip macOS metadata from the stage when packaging on macOS.
5. Create the tarball with `COPYFILE_DISABLE=1`.
6. Verify the tarball by extracting it in a Linux container and running
   `./matilda-prep help` from the extracted project root.
7. For `inventory validate` and `status`, copy a real operator `inventory.yml`
   into the extracted project root first. Packaged tarballs intentionally do not
   include local inventory or `.env` files.

Example macOS packaging pattern for a Linux amd64 tarball:

```bash
version=vX.Y.Z
stage=/private/tmp/matilda-package-${version}-linux-amd64
rm -rf "$stage"
mkdir -p "$stage"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /private/tmp/matilda-prep-linux-amd64 ./cmd/matilda-prep
git archive --format=tar --prefix=matilda-discovery-readiness/ HEAD | tar -xf - -C "$stage"
cp /private/tmp/matilda-prep-linux-amd64 "$stage/matilda-discovery-readiness/matilda-prep"
chmod +x "$stage/matilda-discovery-readiness/matilda-prep"
xattr -cr "$stage/matilda-discovery-readiness" 2>/dev/null || true
COPYFILE_DISABLE=1 tar -czf "dist/matilda-discovery-readiness-${version}-linux-amd64.tar.gz" -C "$stage" matilda-discovery-readiness
```

Write checksums from the asset directory so users can validate downloaded files
without recreating local `dist/...` paths:

```bash
(
  cd dist
  shasum -a 256 \
    matilda-prep-darwin-arm64 \
    matilda-prep-linux-amd64 \
    "matilda-discovery-readiness-${version}-darwin-arm64.tar.gz" \
    "matilda-discovery-readiness-${version}-linux-amd64.tar.gz" \
    > checksums.txt
)
```

Before publishing, verify no macOS metadata is present:

```bash
if tar -tzf "dist/matilda-discovery-readiness-${version}-linux-amd64.tar.gz" | grep -F '._'; then
  echo "macOS metadata found in tarball"
  exit 1
fi
```

Then verify in Podman or another Linux runtime:

```bash
podman run --rm --platform linux/amd64 -v "$PWD:/work:Z" -w /work alpine:latest sh -c 'tar -xzf dist/matilda-discovery-readiness-vX.Y.Z-linux-amd64.tar.gz -C /tmp && cd /tmp/matilda-discovery-readiness && cp /work/inventory.yml inventory.yml && ./matilda-prep help && ./matilda-prep inventory validate && ./matilda-prep status'
```
