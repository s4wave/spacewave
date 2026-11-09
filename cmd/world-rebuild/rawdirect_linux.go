package main

import "syscall"

// oDirect makes raw reads bypass the page cache, so a pass over a live file
// does not evict the backend's cached pages.
const oDirect = syscall.O_DIRECT
