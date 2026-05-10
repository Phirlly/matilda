# Discovery Access Model

Matilda Probe-based discovery uses a Probe inside the customer or cloud environment.

The Probe pulls discovery tasks from Matilda SaaS, connects locally to targets, collects metadata, and sends results outbound over HTTPS/TLS.

Linux and UNIX targets are accessed over SSH. Windows targets are accessed over WinRM/WMI and SMB where required. Cloud and Kubernetes discovery use API-based readiness models.

Preparation may modify target systems. Matilda discovery itself is agentless and read-only.
