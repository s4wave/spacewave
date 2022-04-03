# Podman

> Library of Container targets for Forge.

## Introduction

This controller looks up a Pod Engine on the bus and runs a K8s Pod.

It accepts a kubernetes Pod spec YAML and a list of Hydra UnixFS FUSE world
volumes. The pod spec can use the volumes with either hostPath or the claimName
of persistentVolumeClaim set to the volume name.

Each EngineId in the WorldVolumes list corresponds to a Input of type World. If
the EngineId field is empty, it will default to "world."

Mounts the world volumes to a location on disk with FUSE and replaces the volume
definitions with hostPath references to those locations.

Uses the "podman play kube pod.yaml" command to run the pod spec.
