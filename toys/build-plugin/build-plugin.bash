#!/bin/bash
set -eo pipefail


controllerbus plugin \
              --codegen-dir $(pwd)/codegen \
              -o plugin.cbus.so \
              --no-cleanup \
              compile --build-prefix "demo" -- \
                github.com/aperturerobotics/bldr/toys/bundle
