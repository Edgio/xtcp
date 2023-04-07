#!/bin/bash
#===============================================================================
# This is called by the systemd xtcp unit file as the ExecStart=/home/vagrant/xtcp-opensource/sbin/xtcp_wrapper.bash
#===============================================================================
#
# This wrapper is providing the ability to:
# 1.  Inspect env var xtcp_disable to NOT start xtcp, instead just exiting cleanly, so systemd doesn't consider xtcp failed
# 2.  Starts xtcp with CLI arguments based on env var xtcp_command

#------------------------------------------
# Function to send to standard output and syslog
# This function essentially saves lines of code below
echo_and_syslog () {
	# If the number of positional arguments > 1, then $2 exists and that's the syslog priority level, plus we echo to standard error and exit 1;
	if [ $# -eq 1 ]; then
		# Normal non-error echo to standard output, and syslog level notice
		syslog_priority="local0.notice";
		echo "$1";
		/usr/bin/logger -p "$syslog_priority" "$1";
	else
		# Higher level error echo to standard error, and syslog level per argument
		syslog_priority="$2"; #e.g. "local0.error"
		echo "$1" >&2;
		/usr/bin/logger -p "$syslog_priority" "$1";
	fi
}

#------------------------------------------
# Quickly check if xtcp is running
XTCP_PID="$(/usr/bin/pgrep --full /home/vagrant/xtcp-opensource/bin/xtcp)";
PGREP_STATUS=$?;
echo_and_syslog "$0 line:$LINENO PGREP_STATUS: $PGREP_STATUS, XTCP_PID: $XTCP_PID";

# Logic for pgrep shown here

# Running
# [Thu Aug 20 22:47:57] root@cache17.tla:~# pgrep -f /home/vagrant/xtcp-opensource/bin/xtcp
# 14818
# [Thu Aug 20 22:48:08] root@cache17.tla:~# echo $?
# Not
# 0
# [Thu Aug 20 22:48:09] root@cache17.tla:~# pgrep -f /home/vagrant/xtcp-opensource/bin/xtcp
# [Thu Aug 20 22:48:15] root@cache17.tla:~# echo $?
# 1

# Summary

# Running
# XTCP_PID=="14818"
# PGREP_STATUS==0

# Not running
# XTCP_PID==""
# PGREP_STATUS==

#------------------------------------------
# Read env to see if xtcp should be disabled
XTCP_DISABLED="$(/bin/echo $XTCP_DISABLED)";
echo_and_syslog "$0 line:$LINENO xtcp disabled: $XTCP_DISABLED";

#------------------------------------------
# If xtcp is disabled
if [[ $XTCP_DISABLED -eq 1 ]]; then
	# We are disabled, so check xtcp is running

	# If xtcp is not running, great.  We're disabled and not running.  Perfect.
	if [[ $PGREP_STATUS -eq 1 ]]; then
		echo_and_syslog "$0 line:$LINENO Exiting cleanly because xtcp is disabled by XTCP_DISABLED=1";
		exit 0;
	fi

	# If xtcp is running
	echo_and_syslog "$0 line:$LINENO xtcp is disabled by XTCP_DISABLED, but is running." "local0.error";
	exit 1;
fi

#------------------------------------------
# If xtcp is enabled

#------------------------------------------
# Quickly check xtcp isn't already running.  It shouldn't be but let's check
if [[ $PGREP_STATUS -eq 0 ]]; then
	echo_and_syslog "$0 line:$LINENO xtcp is already running. Please do NOT run xtcp twice!  Likely to be a lot of work for the kernel. pid=$XTCP_PID" "local0.error";
	echo_and_syslog "$0 line:$LINENO Exiting 1" "local0.error";
	exit 1;
else
	echo_and_syslog "$0 line:$LINENO xtcp is NOT already running.  Great.";
fi

#-----------------------------------------------------------------------------------------------------------------
# Read in env vars with some sanity checking

#------------------------------------------
# Read and sanity check xtcp_frequency
#
# Please note we could obviously do a lot better job of checking frequency
# But this is just a few checks to make sure it's vaguely safe to use
#
XTCP_FREQUENCY="$(/bin/echo $XTCP_FREQUENCY)";
echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY: $XTCP_FREQUENCY";

if [[ $XTCP_FREQUENCY == "default" ]]; then
	XTCP_FREQUENCY="30s";
	echo_and_syslog "$0 line:$LINENO Using default XTCP_FREQUENCY: $XTCP_FREQUENCY";
fi

#--------------
# Length can't be < 2 chars.
# "1s"  = 1 second     chars = 2
# "1m"  = 1 minute     chars = 2
XTCP_FREQUENCY_LENGTH="${#XTCP_FREQUENCY}"
echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY_LENGTH: $XTCP_FREQUENCY_LENGTH";
if [[ $XTCP_FREQUENCY_LENGTH -lt 2 ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY cannot be less than 2 chars" "local0.error";
	exit 1;
fi
# Length can't be > 6 chars.
# "900s"   = 15m      chars = 4
# "3600s"  = 1hour    chars = 5
# "86400s" = 24 hours chars = 6
if [[ $XTCP_FREQUENCY_LENGTH -gt 6 ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY cannot be more than 5 chars" "local0.error";
	exit 1;
fi

#--------------
# Check non-last is numeric
XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR=${XTCP_FREQUENCY::(-1)}
echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR:$XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR";
if [[ ! "$XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR" =~ ^[0-9]+$ ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY must be numeric" "local0.error";
	exit 1;
fi
XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR_NUM="$((XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR + 0))"
#--------------
# Numbers must be < 86400
if [[ $XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR_NUM -gt 86400 ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY must be <=86400 somethings:$XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR_NUM" "local0.error";
	exit 1;
fi
#--------------
# Numbers must be > 0
if [[ $XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR_NUM -lt 1 ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY must be >0 somethings:$XTCP_FREQUENCY_EVERYTHING_BUT_LAST_CHAR_NUM" "local0.error";
	exit 1;
fi

# Check last character is s,m, or h
XTCP_FREQUENCY_LAST_CHAR=${XTCP_FREQUENCY:(-1)}
# seconds, minutes, hours
if [[ ! $XTCP_FREQUENCY_LAST_CHAR =~ ^(s|m|h)$ ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FREQUENCY must end with s, m, or h:$XTCP_FREQUENCY_LAST_CHAR" "local0.error";
	exit 1;
fi

#------------------------------------------
# Read and sanity check xtcp_sampling_modulus
#
XTCP_SAMPLING_MODULUS="$(/bin/echo $XTCP_SAMPLING_MODULUS)";
echo_and_syslog "$0 line:$LINENO XTCP_SAMPLING_MODULUS: $XTCP_SAMPLING_MODULUS";

if [[ $XTCP_SAMPLING_MODULUS == "default" ]]; then
	XTCP_SAMPLING_MODULUS=2;
	echo_and_syslog "$0 line:$LINENO Using default XTCP_SAMPLING_MODULUS: $XTCP_SAMPLING_MODULUS";
fi

#--------------
# Check modulus is only numeric
if [[ ! "$XTCP_SAMPLING_MODULUS" =~ ^[0-9]+$ ]]; then
	echo_and_syslog "$0 line:$LINENO XTCP_SAMPLING_MODULUS must be numeric" "local0.error";
	exit 1;
fi
#--------------
# Check sampling modulus isn't very high
# We normally want to run with sampling modulus of either 1 or 2 value, but maybe we want it higher?
# If we really want it higher, we can roll a new bundle.
if [ $XTCP_SAMPLING_MODULUS -ge 10 ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_SAMPLING_MODULUS must <= 10:$XTCP_SAMPLING_MODULUS" "local0.error";
	exit 1;
fi
# Must be greater than zero >0
if [ $XTCP_SAMPLING_MODULUS -lt 1 ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_SAMPLING_MODULUS must >= 1:$XTCP_SAMPLING_MODULUS" "local0.error";
	exit 1;
fi


#------------------------------------------
# Read and sanity check xtcp_report_modulus
#
XTCP_REPORT_MODULUS=$(/bin/echo $XTCP_REPORT_MODULUS);
echo_and_syslog "$0 line:$LINENO XTCP_REPORT_MODULUS: $XTCP_REPORT_MODULUS";

if [[ $XTCP_REPORT_MODULUS == "default" ]]; then
	XTCP_REPORT_MODULUS=2000;
	echo_and_syslog "$0 line:$LINENO Using default XTCP_REPORT_MODULUS: $XTCP_REPORT_MODULUS";
fi

#--------------
# Check modulus is only numeric
if [ $XTCP_REPORT_MODULUS -ne $XTCP_REPORT_MODULUS ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_REPORT_MODULUS must be numeric:$XTCP_REPORT_MODULUS " "local0.error";
	exit 1;
fi
#--------------
# Check report modulus isn't very high
if [ $XTCP_REPORT_MODULUS -ge 10000 ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_REPORT_MODULUS must <= 10,000:$XTCP_REPORT_MODULUS" "local0.error";
	exit 1;
fi
# Must be greater than zero >0
if [ $XTCP_REPORT_MODULUS -lt 1 ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_REPORT_MODULUS must >= 1:$XTCP_REPORT_MODULUS" "local0.error";
	exit 1;
fi

#------------------------------------------
# Read and sanity check xtcp_filter_report_modulus
#
XTCP_FILTER_REPORT_MODULUS="$(/bin/echo $XTCP_REPORT_MODULUS)";
echo_and_syslog "$0 line:$LINENO XTCP_FILTER_REPORT_MODULUS: $XTCP_FILTER_REPORT_MODULUS";

if [[ $XTCP_FILTER_REPORT_MODULUS == "default" ]]; then
	XTCP_FILTER_REPORT_MODULUS=2000;
	echo_and_syslog "$0 line:$LINENO Using default XTCP_FILTER_REPORT_MODULUS: $XTCP_FILTER_REPORT_MODULUS";
fi

#--------------
# Check modulus is only numeric
if [ $XTCP_FILTER_REPORT_MODULUS -ne $XTCP_FILTER_REPORT_MODULUS ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FILTER_REPORT_MODULUS must be numeric:$XTCP_FILTER_REPORT_MODULUS " "local0.error";
	exit 1;
fi
#--------------
# The filter modulus may be quite high, so we skip the max value check.
# Must be greater than zero >0
if [ $XTCP_FILTER_REPORT_MODULUS -lt 1 ]; then
	echo_and_syslog "$0 line:$LINENO XTCP_FILTER_REPORT_MODULUS must >= 1:$XTCP_FILTER_REPORT_MODULUS" "local0.error";
	exit 1;
fi

#------------------------------------------
# Load the list of pop-local IPs from pops.json
XTCP_FILTER_JSON="$(/bin/echo $XTCP_FILTER_JSON)";

#------------------------------------------
# Read in the pop name
XTCP_FILTER_GROUP="$(/bin/echo $XTCP_FILTER_GROUP)";

#------------------------------------------
# Read env var to see if fitlering is enabled
XTCP_ENABLE_FILTER="$(/bin/echo $XTCP_ENABLE_FILTER)";
echo_and_syslog "$0 line:$LINENO xtcp filter enabled: $XTCP_ENABLE_FILTER";

#NSQ
XTCP_NSQ=$(/bin/echo $XTCP_NSQ);
echo_and_syslog "$0 line:$LINENO xtcp nsq: $XTCP_NSQ";

# END - Read in env vars with some sanity checking
#-----------------------------------------------------------------------------------------------------------------

#------------------------------------------
# Build up the exec command
# It should look something like this:
# "/home/vagrant/xtcp-opensource/bundle/bin/xtcp -frequency 30s -samplingModulus 2 -inetdiagerReportModulus 2000";
XTCP_BASE_COMMAND="/home/vagrant/xtcp-opensource/bundle/bin/xtcp";
echo_and_syslog "$0 line:$LINENO XTCP_BASE_COMMAND: $XTCP_BASE_COMMAND";

EXEC_COMMAND_ARRAY[0]="$XTCP_BASE_COMMAND";
EXEC_COMMAND_ARRAY[1]="-frequency";
EXEC_COMMAND_ARRAY[2]="$XTCP_FREQUENCY";
EXEC_COMMAND_ARRAY[3]="-samplingModulus";
EXEC_COMMAND_ARRAY[4]="$XTCP_SAMPLING_MODULUS";
EXEC_COMMAND_ARRAY[5]="-inetdiagerReportModulus";
EXEC_COMMAND_ARRAY[6]="$XTCP_REPORT_MODULUS";

# Counter to keep track of optional indices
ind=7

if [[ $XTCP_ENABLE_FILTER != "" ]]; then
	EXEC_COMMAND_ARRAY[7]="-enableFilter";
	EXEC_COMMAND_ARRAY[8]="-inetdiagerFilterReportModulus";
	EXEC_COMMAND_ARRAY[9]="$XTCP_FILTER_REPORT_MODULUS";
	EXEC_COMMAND_ARRAY[10]="-filterJson";
	EXEC_COMMAND_ARRAY[11]="$XTCP_FILTER_JSON";
	EXEC_COMMAND_ARRAY[12]="-filterGroup";
	EXEC_COMMAND_ARRAY[13]="$XTCP_FILTER_GROUP";
	ind=14
fi


if [[ $XTCP_NSQ != "" ]]; then
	EXEC_COMMAND_ARRAY[$ind]="-nsq";
	EXEC_COMMAND_ARRAY[$((ind+1))]="$XTCP_NSQ";
fi

# Print out what we have.
for PART in "${EXEC_COMMAND_ARRAY[@]}"; do
	echo_and_syslog "$0 line:$LINENO EXEC_COMMAND_ARRAY PART:$PART}" "local0.error";
done

if [ "$1" == "test" ]; then
	echo_and_syslog "$0 line:$LINENO Command line argument is 'test', so finishing before exect";
	exit 0;
fi

#------------------------------------------
# Finally, let's try to run this thing!!
exec "${EXEC_COMMAND_ARRAY[@]}";
