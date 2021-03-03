#!/bin/bash

set -euo pipefail
# set -euxo pipefail

clear
source ./env.sh


if [ -d ~/.genesis-sectors ];then
	    rm -rf ~/.genesis-sectors
fi
if [ -d ${REPO_PATH} ];then
	    rm -rf ${REPO_PATH}
fi
if [ -d ${DATA_PATH} ];then
	    rm -rf ${DATA_PATH}
fi
mkdir ${DATA_PATH}


lotus-seed pre-seal --sector-size ${SECTOR_SIZE} --num-sectors ${SECTOR_NUM} && \
	lotus-seed genesis new ${DATA_PATH}/localnet.json && \
	lotus-seed genesis add-miner ${DATA_PATH}/localnet.json ~/.genesis-sectors/pre-seal-t01000.json

# lotus daemon --api=7001 --lotus-make-genesis=${DATA_PATH}/dev.gen --genesis-template=${DATA_PATH}/localnet.json --bootstrap=false 2>&1 | tee ${DATA_PATH}/lotus.log
lotus daemon --api=60010 --lotus-make-genesis=${DATA_PATH}/dev.gen --genesis-template=${DATA_PATH}/localnet.json --bootstrap=false > ${DATA_PATH}/lotus.log 2>&1 &
DAEMON_PID=$!


lotus wait-api
lotus wallet import ~/.genesis-sectors/pre-seal-t01000.key && \
	lotus wallet set-default `lotus wallet list | grep -E "t3|f3" | awk '{print $1}'`  && \
	tail -fn 100 ${DATA_PATH}/lotus.log
wait $DAEMON_PID
