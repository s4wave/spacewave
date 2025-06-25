# Plugin Host Scheduler

This controller manages selecting which manifest to execute for each plugin and
executing the plugins. It also manages attempting to fully download copies of
new plugin versions before replacing the old plugin version instances to avoid
interruptions in the User Experience.

## Overview of Routines

Here are the routines managed by the scheduler:

Global, for all plugins:

- Task 0: Watch the list of available plugin hosts with CollectValues (Execute())

Managed by the pluginInstance for each plugin-id we want to execute:

- Task 1: FetchManifests: call FetchManifest<plugin-id> with platform id set to any
  - Unless the DisableFetchManifests option is set.
  - Write the ManifestRefs into the world if newer than the latest (or not exist) in one transaction
  - We will track all available platform manifests for the plugin id in the world,

- Task 2: SelectManifest: select the best manifest(s) from the set available in the world.
  - In a world transaction: query the list of available manifests for the plugin id
    - search for Manifests or ManifestRefs
  - Drop any manifests that are for platform IDs we don't have hosts for
  - Sort by rev, followed by preferred platform ID to less preferred platform ID
  - Iterate over the list and identify two manifests from the set of available:
    - Download manifest: the first manifest in the list.
    - Live manifest: the first manifest in the list which has been fully downloaded.
    - Fallback to the download manifest for the Live manifest if none are downloaded.
  - Set the Download manifest to the DownloadManifest state routine
  - Set the Live manifest to the ExecutePlugin state routine

- Task 3: DownloadManifest: download the "Download manifest" to storage fully
  - Traverse all the blocks in the block graph
  - Any block not present in local storage => fetch and copy to local storage
  - Mark the manifest as fully downloaded once complete.

- Task 5: ExecutePlugin: execute the plugin with a ManifestSnapshot
  - Mount the plugin fs from the world block graph
  - Execute the plugin
