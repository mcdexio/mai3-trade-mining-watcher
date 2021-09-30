#!/bin/bash

# env
export CI=false
export LOG_COLOR=true
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
export MAI3_TRADE_MINING_GRAPH_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/mai3-trading-mining2
export BLOCKS_GRAPH_URL=https://api.thegraph.com/subgraphs/name/renpu-mcarlo/arbitrum-rinkeby-blocks

# setting
export DEFAULT_EPOCH_0_START_TIME=1632972600
export RESET_DATABASE=true
export SYNC_DELAY=400

# inverse white list
export COUNT_INVERSE_CONTRACT_WHITELIST=2
export INVERSE_CONTRACT_WHITELIST0='0x3d3744dc7a17d757a2568ddb171d162a7e12f80-0'
export INVERSE_CONTRACT_WHITELIST1='0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-0'
