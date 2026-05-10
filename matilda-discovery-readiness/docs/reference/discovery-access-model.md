# Discovery Access Model

Matilda Probe-based discovery uses a Probe inside the customer or cloud environment.

The Probe pulls discovery tasks from Matilda SaaS, connects locally to targets, collects metadata, and sends results outbound over HTTPS/TLS.

The current toolkit automates Linux target readiness. Linux targets are accessed over SSH either directly from the operator machine or through MatildaProbeVM.

Other access models are for planning only in this release candidate:

- UNIX targets use SSH-based planning guidance.
- Windows targets use generated readiness package guidance.
- Cloud and Kubernetes readiness are not automated in this release candidate.

Preparation may modify target systems. Matilda discovery itself is agentless and read-only.
