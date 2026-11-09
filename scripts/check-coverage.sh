#!/bin/sh
set -eu
go test -coverprofile=cover.out ./internal/...
total=$(go tool cover -func=cover.out | awk '/^total:/ {print $3}' | tr -d '%')
awk -v t="$total" 'BEGIN { if (t < 75) { print "total coverage " t "% < 75%"; exit 1 } }'
go test -coverprofile=cover-domain.out ./internal/domainlist
dom=$(go tool cover -func=cover-domain.out | awk '/^total:/ {print $3}' | tr -d '%')
awk -v d="$dom" 'BEGIN { if (d < 80) { print "domainlist coverage " d "% < 80%"; exit 1 } }'
