// Forge graph predicates for entity relationships.
// These match the Go-side constants in the forge packages.
export const PRED_CLUSTER_TO_JOB = '<forge/cluster-job>'
export const PRED_CLUSTER_TO_WORKER = '<forge/cluster-worker>'
export const PRED_JOB_TO_TASK = '<forge/job-task>'
export const PRED_TASK_TO_PASS = '<forge/task-pass>'
export const PRED_TASK_TO_SUBTASK = '<forge/task-subtask>'
export const PRED_TASK_TO_CACHED = '<forge/task-cached>'
export const PRED_PASS_TO_EXECUTION = '<forge/pass-execution>'
export const PRED_DASHBOARD_FORGE_REF = '<dashboard/forge-ref>'
export const PRED_OBJECT_TO_KEYPAIR = '<identity/keypair-link>'
