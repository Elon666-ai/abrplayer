#!/usr/bin/env bash
set -euo pipefail
go build -ldflags="-s -w -X 'main.BuildTime=`date +'%Y-%m-%d %H:%M:%S'`'" -o bin/abrplayer-backend.exe main.go
