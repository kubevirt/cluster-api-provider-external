# cluster-api-provider-external
An implementation of the Cluster Management API that delegates CRUD primitives to an admin-defined container.

Currently there are two container options, 
one that is a thin wrapper around the RHEL-HA fencing agents for those focussed exclusively on power management;
and one based around Ansible playbooks that could be extended to interface with bespoke provisioning systems.
