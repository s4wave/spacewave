#!/bin/sh
set -e

npm i -g yarn
yarn install
yarn run build
