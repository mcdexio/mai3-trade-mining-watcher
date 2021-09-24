#!/bin/bash


# env
export CI=false

export SERVER_LOG_TO_STACKDRIVER=false
export SERVER_LOG_TO_STDOUT=true
export SERVER_PROJECT_ID=mai3-trade-mining-watcher
export SERVER_LOGLEVEL=6
export HOSTNAME=localhost

# DB parameters
export DB_MAX_IDLE_CONNS=-1
export DB_MAX_OPEN_CONNS=16
export DB_ARGS=postgres://mcdex@localhost:5432/mcdex?sslmode=disable # Disable ssl for now

# Graph URL
export MAI3_TRADE_MINING_URL=https://api.thegraph.com/subgraphs/name/mcdexio/mai3-trade-mining
export MAI3_PERPETUAL_URL=https://api.thegraph.com/subgraphs/name/mcdexio/mcdex3-arb-perpetual2
export MAI3_STAKE_URL

# setting
export INTERVAL_SECOND=60
