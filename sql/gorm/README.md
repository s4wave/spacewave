# Go-ORM Engine

This package adapts the code from the [Go-orm MySQL driver] to run on top of any
of the Hydra SQL stores, including GenjiDB and go-mysql-server.

[Go-orm MySQL driver]: https://github.com/go-gorm/mysql/

## GenjiDB Note

Note: GenjiDB is not yet at v1.0.0 and does not support the full sql dialect, or
even the baseline features of SQL. For that reason it is primarily a tech-demo /
work-in-progress and shouldn't be used for anything important (yet).
