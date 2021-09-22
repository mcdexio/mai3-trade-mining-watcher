#!/bin/bash

GITROOT=$(git rev-parse --show-toplevel)

export PATH=$GITROOT/bin:$PATH

if [ $# -eq 0 ]
  then
    echo "No arguments supplied, use default config.sh"
    CONFIG_FILE=$GITROOT/bin/config.sh
else
    CONFIG_FILE=$GITROOT/bin/$1.sh
    echo "Use config $CONFIG_FILE"
fi

if [ -r "$CONFIG_FILE" ]
  then
    echo "source $CONFIG_FILE"
    source $CONFIG_FILE
else
    echo "Config $CONFIG_FILE is not exist."
fi
