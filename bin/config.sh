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

# setting
export DEFAULT_EPOCH_0_START_TIME=1634058000
export RESET_DATABASE=true
export SYNC_DELAY=0

# Graph URL arb-rinkeby & bsc
export MAI3_TRADE_MINING_GRAPH_ARB_URL=https://api.thegraph.com/subgraphs/name/champfu-mcdex/mai3-trading-mining2
export BLOCKS_GRAPH_ARB_URL=https://api.thegraph.com/subgraphs/name/renpu-mcarlo/arbitrum-rinkeby-blocks
export MAI3_TRADE_MINING_GRAPH_BSC_URL=https://api.thegraph.com/subgraphs/name/mcdexio/mcdex3-bsc-trade-mining
export BLOCKS_GRAPH_BSC_URL=https://api.thegraph.com/subgraphs/name/generatefinance/bsc-blocks

# inverse white list arb-rinkeby
export BTC_COUNT_INVERSE_CONTRACT_WHITELIST=2
export BTC_INVERSE_CONTRACT_WHITELIST0='0x3d3744dc7a17d757a2568ddb171d162a7e12f80a-0' # arb-rinkeby USD-BTC
export BTC_INVERSE_CONTRACT_WHITELIST1='0x2ea001032b0eb424120b4dec51bf02db0df46c78-0' # bsc USD-BTC
export ETH_COUNT_INVERSE_CONTRACT_WHITELIST=3
export ETH_INVERSE_CONTRACT_WHITELIST0='0x727e5a9a04080741cbc8a2dc891e28ca8af6537e-0' # USD-ETHB
export ETH_INVERSE_CONTRACT_WHITELIST1='0xf6b2d76c248af20009188139660a516e5c4e0532-0' # bsc USD-ETH
export ETH_INVERSE_CONTRACT_WHITELIST2='0xf6b2d76c248af20009188139660a516e5c4e0532-1' # bsc BTC-ETH
