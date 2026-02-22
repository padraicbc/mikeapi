#!/bin/bash
set -euo pipefail

cd ../mikesvelte
pnpm build
rm -rf ../mikeapi/build
cp -r build ../mikeapi/build

cd ../mikeapi
go generate ./...
CGO_ENABLED=0 go build -o api

scp api padraic@$MIKEDO:/home/padraic/app
scp -r docker padraic@$MIKEDO:/home/padraic/app
