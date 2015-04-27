#!/bin/sh

FIFO="/home/xena/.local/share/within/status/fifos/$$"

[ -e "$FIFO" ] || mkfifo "$FIFO"
chmod 600 $FIFO

dvtm -s $FIFO "$@" 2> /dev/null

rm $FIFO
