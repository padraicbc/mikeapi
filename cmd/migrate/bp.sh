#!/bin/bash

CGO_ENABLED=0 go build -o migrate
scp migrate  padraic@$MIKEDO:/home/padraic/app
