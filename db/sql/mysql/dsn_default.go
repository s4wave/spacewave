//go:build !tinygo

package mysql

import mysql2 "github.com/go-sql-driver/mysql"

func parseDSN(dsn string) (string, error) {
	cfg, err := mysql2.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	return cfg.DBName, nil
}
