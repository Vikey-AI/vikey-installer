#!/usr/bin/env bash
# Usage: annotate-failure.sh <title> <log file>
# Turns the end of a log into one GitHub Actions error annotation. Job logs
# need admin rights to read, annotations are public on a public repository,
# so failures stay debuggable from outside.
title=$1
log=$2
[ -f "$log" ] || exit 0
body=$(tail -n 80 "$log" | sed -e 's/\x1b\[[0-9;]*[A-Za-z]//g' | sed -e 's/%/%25/g' -e 's/\r/%0D/g' | awk '{printf "%s%%0A", $0}')
echo "::error title=$title::$body"
