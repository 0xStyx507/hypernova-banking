#!/bin/sh
set -eu

data_file=/data/0_0.tigerbeetle

# Format the single-node development volume only on first startup. Reusing an
# existing file preserves the ledger between container restarts.
if [ ! -f "$data_file" ]; then
  tigerbeetle format --cluster=0 --replica=0 --replica-count=1 "$data_file"
fi

exec tigerbeetle start --development --addresses=0.0.0.0:3000 "$data_file"
