#!/bin/bash


# env
export CI=false

export SERVER_LOG_TO_STACKDRIVER=false
export SERVER_LOG_TO_STDOUT=true
export SERVER_PROJECT_ID=mai3-trade-mining-watcher
export SERVER_LOGLEVEL=5
export HOSTNAME=localhost

# DB parameters
export DB_MAX_IDLE_CONNS=-1
export DB_MAX_OPEN_CONNS=16
export DB_ARGS=postgres://mcdex@localhost:5432/mcdex?sslmode=disable # Disable ssl for now

# Graph URL
export MAI3_TRADE_MINING_GRAPH_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/mai3-trading-mining2
# export ARB_BLOCKS_GRAPH_URL=https://api.thegraph.com/subgraphs/name/ianlapham/arbitrum-one-blocks
export ARB_BLOCKS_GRAPH_URL=https://api.thegraph.com/subgraphs/name/renpu-mcarlo/arbitrum-rinkeby-blocks

# setting
export SYNCER_BLOCK_START_TIME="2021-09-26T01:00:00+08:00"
