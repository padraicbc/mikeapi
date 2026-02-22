#!/bin/bash

CGO_ENABLED=0 go build -o adduser
scp adduser  padraic@$MIKEDO:/home/padraic/app
