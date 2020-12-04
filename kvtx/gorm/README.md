# Go ORM Engine

This package uses the WIP GenjiDB package to build a database/sql compatible
store with a kvtx store, and then build a go-orm database on top of that.

This package leverages GenjiDB + Go Orm (gorm) to build a database/sql
compatible store on top of a kvtx-compatible store.

Note: GenjiDB is not yet at v1.0.0 and does not support the full sql dialect, or
even the baseline features of SQL. For that reason this package is primarily a
tech-demo / work-in-progress and shouldn't be used for anything important (yet).

