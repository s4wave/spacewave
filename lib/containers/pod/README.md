# Execute Pod

> Execute a pod with volumes mounted from the World.

## Introduction

Configures or deletes a volume and/or HostVolume world object.

Mounts the world volumes to a location on disk with FUSE and replaces the volume
definitions with hostPath references to those locations.

## Podman

Uses the "podman play kube pod.yaml" command to run the pod spec.
