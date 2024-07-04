#!/bin/bash

# Set the output directory
OUT_DIR="bin"

mkdir -p $OUT_DIR

# Cross-compile for Mac
env GOOS=darwin GOARCH=amd64 go build -v -x -o $OUT_DIR/ff_scrape_earnings-mac64 main.go

# Cross-compile for Windows.
env GOOS=windows GOARCH=amd64 go build -v -x -o $OUT_DIR/ff_scrape_earnings-win64.exe main.go

# Cross-compile for Linux.
env GOOS=linux GOARCH=amd64 go build -v -x -o $OUT_DIR/ff_scrape_earnings-linux64 main.go
