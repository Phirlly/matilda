# Release Candidate Baseline

This release manager reference defines the release candidate scope for Matilda Discovery Readiness Toolkit. Operators should start with the root [README](../../README.md) or [Quickstart](../user/quickstart.md).

## Supported Today

- Linux target readiness for targets reached directly from the operator machine.
- Linux target readiness for targets reached through MatildaProbeVM.
- Default `inventory.yml` and normalized v1 inventory for Linux execution.
- Local terminal console and local browser UI.
- Local run history under `.matilda/`.
- Readiness reports under `reports/`.

## Validated Workflows

The release candidate baseline validates these Linux workflows:

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

Windows remote automation and UNIX remote automation are not implemented in this release candidate.

## Not Automated

These areas are not automated in this release candidate:

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

Windows operator machines are not validated in this release candidate. Use WSL only if Go, Ansible, and SSH are configured there.

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
- Prebuilt release binaries are operating-system and CPU-architecture specific. A source clone can be used on validated operator platforms with Go installed.

## Branch And Tag Workflow

Release candidate changes must be completed on `featureBranch` or another non-main branch first.

1. Make changes on the non-main branch.
2. Run validation on that branch.
3. Push the branch for review.
4. Merge or fast-forward `main` only after checks pass.
5. Create release tags from `main`.
6. After a release is published, prefer a new RC tag over moving the published tag unless the tag move is explicitly approved.

## Release Candidate Validation

Before tagging an RC, run:

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
