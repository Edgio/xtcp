#!/bin/bash

# Short bash script for testing the xtcp disabler function
#
# This script increments the value in a temp file and returns 1
# when the value of the counter is greater than $2
#
# ./testdata/return_one_after_X_runs.bash --default=0 5
#
if [ ! -f ./testdata/tmp_counter ]; then
	echo 0 > ./testdata/tmp_counter;
fi
COUNTER=$(/bin/cat ./testdata/tmp_counter);
if [[ $COUNTER -gt $2 ]]; then
	echo "1"
	exit 0
fi
(( COUNTER ++ ))
echo "$COUNTER" > ./testdata/tmp_counter
echo "0"
