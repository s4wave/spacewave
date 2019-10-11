Some useful notes from the Badger repository README:

# Memory Usage

Badger's memory usage can be managed by tweaking several options available in
the `Options` struct that is passed in when opening the database using
`DB.Open`.

- `Options.ValueLogLoadingMode` can be set to `options.FileIO` (instead of the
  default `options.MemoryMap`) to avoid memory-mapping log files. This can be
  useful in environments with low RAM.
- Number of memtables (`Options.NumMemtables`)
  - If you modify `Options.NumMemtables`, also adjust
   `Options.NumLevelZeroTables` and `Options.NumLevelZeroTablesStall`
   accordingly.
- Number of concurrent compactions (`Options.NumCompactors`)
- Mode in which LSM tree is loaded (`Options.TableLoadingMode`)
- Size of table (`Options.MaxTableSize`)
- Size of value log file (`Options.ValueLogFileSize`)

If you want to decrease the memory usage of Badger instance, tweak these options
(ideally one at a time) until you achieve the desired memory usage.
