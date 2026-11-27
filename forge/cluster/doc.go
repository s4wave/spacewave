// Package forge_cluster defines the Cluster, which schedules Jobs onto the
// Workers linked to it.
//
// A Cluster stores a DNS-label name and the peer that runs its scheduler, and
// links to its Jobs with PredClusterToJob and its Workers with
// PredClusterToWorker. Its typed operations create it, assign its peer, add
// Jobs and Workers, start a Job, assign a Task, and complete a Job. Only the
// assigned peer may submit the scheduling operations. Several Clusters may serve
// one Job, each Execution going to one Worker of one of them, so a Job spreads
// across Clusters without splitting its World state.
package forge_cluster
