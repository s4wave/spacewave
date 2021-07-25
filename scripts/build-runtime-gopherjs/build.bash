#!/bin/bash
set -eo pipefail

# Weird quirk - logrus is not recognized in vendor sometimes.
mv ../../vendor/github.com/sirupsen $GOPATH/src/github.com
go run -mod=vendor -v ./
