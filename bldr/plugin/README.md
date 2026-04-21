# Plugins

Plugins communicate using [staRPC] services.

The **host** manages loading plugins, and exposes an API for plugins to declare
dependencies on other plugins and to update plugin binaries.

The **root** plugin is the initial plugin loaded by the **host**. It should
expose a RPC service which the host calls to resolve plugin binaries on-demand.

[staRPC]: https://github.com/aperturerobotics/starpc
