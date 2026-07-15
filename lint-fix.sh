#!/bin/bash
go clean -modcache
go mod tidy
golangci-lint run
