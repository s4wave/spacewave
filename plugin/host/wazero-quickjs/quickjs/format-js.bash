#!/bin/bash
set -eo pipefail

prettier -w --semi ./*.js
