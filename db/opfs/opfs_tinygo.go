//go:build js && tinygo

package opfs

// Exported OPFS helper callbacks publish completion only. JavaScript owns the
// later TinyGo scheduler resume so resumed goroutines do not enter syscall/js
// while a js.FuncOf callback frame is still active.

//go:wasmexport BLDR_OPFS_HELPER_RESOLVE
func tinygoOPFSHelperResolve(opID uint32, count uint32, value0 uint32, value1 uint32) {
	values := make([]int, 0, count)
	if count > 0 {
		values = append(values, int(value0))
	}
	if count > 1 {
		values = append(values, int(value1))
	}
	completeOPFSHelperOp(int(opID), opfsHelperResult{values: values})
}

//go:wasmexport BLDR_OPFS_HELPER_REJECT
func tinygoOPFSHelperReject(opID uint32, code uint32) {
	completeOPFSHelperOp(int(opID), opfsHelperResult{err: newJSErrorCode(int(code))})
}
