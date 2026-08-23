package s4wave_kv_world

var (
	_ KVStore      = (*WorldBackedStore)(nil)
	_ WatchKVStore = (*WorldBackedStore)(nil)
	_ KVStore      = (*RemoteStore)(nil)
	_ WatchKVStore = (*RemoteStore)(nil)
)
