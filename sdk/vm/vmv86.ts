// VmV86TypeID is the object type identifier for VmV86 objects. It mirrors
// VmV86TypeID in sdk/vm/v86.go.
export const VmV86TypeID = 'vm/v86'

// CREATE_VM_V86_OP_ID is the operation type ID for CreateVmV86Op.
export const CREATE_VM_V86_OP_ID = 'vm/v86/create'

// SET_V86_STATE_OP_ID is the operation type ID for SetV86StateOp.
export const SET_V86_STATE_OP_ID = 'vm/v86/set-state'

// PRED_V86_IMAGE is the graph predicate linking a VmV86 to its V86Image.
// It mirrors PredV86Image in sdk/vm/v86.go, which stores the bare IRI without
// angle brackets.
export const PRED_V86_IMAGE = 'v86/image'
