// Package inetdiagerstater is the inetdiager stats go routine as part of the xtcp package
//
// Basically, this go routine recieves stats over the channel and updates Prometheus and statsd stats
package inetdiagerstater

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	debugLevel int = 11
)

// TODO move to misc
var (
	// Map kernel enum to human string for the IP protocol
	kernelEnumToString = map[uint8]string{
		uint8(2):  "v4",
		uint8(10): "v6",
	}
)

// InetdiagerStatsWrapper struct has AF, id, and then the inetdiagerStats
type InetdiagerStatsWrapper struct {
	Af    uint8
	ID    int
	Stats InetdiagerStats
}

// InetdiagerStats struct are the interesting stats coming out of each inetdiager
type InetdiagerStats struct {
	InetdiagMsgInSizeTotal    int
	InetdiagMsgCount          int
	InetdiagMsgFilterCount    int
	InetdiagMsgBytesReadTotal int
	PadBufferTotal            int
	UDPWritesTotal            int
	UDPBytesWrittenTotal      int
	UDPErrorsTotal            int
	StatsBlocked              int
}

// InetdiagerStater calculates stats for the inetdiagers
// The inetdiagers run a timer that when it expires the inetdiager will send summary stats over the channel to inetdiagerStater
// Please note that the timer is currnetly half (1/2) the polling frequency.  PLEASE BE CAREFUL WHEN DECREASING THE POLLING FREQUENCY
// TODO Consider an alternative strategy where the poller could signal the inetdiagers via a channel that the polling cycle is done,
// and then inetdiagers could start a timed loop to check for no more messages, and then report.  This would help find when the polling is done,
// and all the messages from the netlinkers have been processed.
func InetdiagerStater(in <-chan InetdiagerStatsWrapper, cliFlags cliflags.CliFlags) {

	//---------------------------------
	// Register Prometheus metrics

	//------------------------------
	// inetdiager static stuff
	inetdiagerReportModulus := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "report_modulus",
			Help:      "inetdiager ReportModulus will sample every X inetdiag messages to send to Kafka.",
		},
	)
	inetdiagerReportModulus.Set(float64(*cliFlags.InetdiagerReportModulus))

	inetdiagerFilterReportModulus := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "filter_report_modulus",
			Help:      "inetdiager FilterReportModulus will sample every X inetdiag messages which match the block filters to send to Kafka.",
		},
	)
	inetdiagerFilterReportModulus.Set(float64(*cliFlags.InetdiagerFilterReportModulus))

	inetdiagerStatsRatio := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "stats_ratio",
			Help:      "inetdiagerStatsRatio controls the how often the inetdiagers send summary stats, which is as a percentage of the pollingFrequencySeconds.",
		},
	)
	inetdiagerStatsRatio.Set(*cliFlags.InetdiagerStatsRatio)

	//------------------------------
	// inetdiager
	inetdiagerIn := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "in",
			Help:      "inetdiager INput bytes from the netlinker channel, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerMsgs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "msgs",
			Help:      "inetdiager messages read from the netlinker channel, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerMsgFilters := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "msgsFilter",
			Help:      "inetdiager messages read from the netlinker channel, by address family, by worker id that matched the filter",
		},
		[]string{"af", "id"},
	)
	inetdiagerRead := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "read",
			Help:      "inetdiager buffer bytes read, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerPad := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "pad",
			Help:      "inetdiager pad buffer bytes read, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerUDPs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "udps",
			Help:      "inetdiager UDP messages sent (likely to be the number of packets), by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerUDPBytes := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "udp_bytes",
			Help:      "inetdiager UDP bytes sent, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerUDPErrors := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "udp_errors",
			Help:      "inetdiager UDP errors on udpConn.Write(udpBytes), by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	inetdiagerStatsBlocked := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "stats_blocked",
			Help:      "inetdiager stats channel blocked, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	//-----
	// Totals for all inetdiagers in the address family
	inetdiagerMsgsTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "msgs_total",
			Help:      "inetdiager total messages read from the netlinker channel, by address family",
		},
		[]string{"af"},
	)
	inetdiagerUDPsTotal := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiager",
			Name:      "udps_total",
			Help:      "inetdiager total UDP messages sent (likely to be the number of packets), by address family",
		},
		[]string{"af"},
	)

	//-------------------
	// inetdiagerStater prometheus counters
	// Please note, these are NOT being sent to statsd
	inetdiagerStaterMsg := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiagerStater",
			Name:      "msgs",
			Help:      "inetdiagerStater messages recieved on the channel, by address family, by id",
		},
		[]string{"af"},
	)
	inetdiagerStaterUDPs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiagerStater",
			Name:      "udps",
			Help:      "inetdiagerStater UDP messages sent (likely to be packets), by address family, by id",
		},
		[]string{"af"},
	)
	inetdiagerStaterUDPBytes := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiagerStater",
			Name:      "udp_bytes",
			Help:      "inetdiagerStater UDP bytes sent, by address family, by id",
		},
		[]string{"af"},
	)
	inetdiagerStaterUDPErrors := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "inetdiagerStater",
			Name:      "udp_errors",
			Help:      "inetdiagerStater UDP messages send errors, by address family, by id",
		},
		[]string{"af"},
	)

	//---------------------------------------------------------
	// Open UDP socket for statsd
	var udpConn net.Conn
	var dialerr error
	var updateString string
	var udpBytesWritten int
	var udpWriteErr error
	// if statsd is enabled
	if *cliFlags.NoStatsd == false {
		udpConn, dialerr = net.Dial("udp", *cliFlags.StatsdDst)
		if dialerr != nil {
			if debugLevel > 10 {
				fmt.Println("pollerStater:\tnet.Dial(\"udp\", ", *cliFlags.StatsdDst, ") error:", dialerr)
			}
		}
		defer udpConn.Close()
	}

	// AF,id -> stats
	var oldStatsMap map[uint8]map[int]InetdiagerStats
	oldStatsMap = make(map[uint8]map[int]InetdiagerStats)
	// Create the top level of the map - TODO iterate
	oldStatsMap[uint8(2)] = make(map[int]InetdiagerStats)
	oldStatsMap[uint8(10)] = make(map[int]InetdiagerStats)
	var diffStats InetdiagerStats

	// Keep our own local total, for output to stdout
	var totalInetdiagerMsgs int
	var totalInetdiagerUDPs int
	// Need this to work out the correct modulus for outputting to stdout
	var afToInetdiagers = map[uint8]*int{
		uint8(2):  cliFlags.Inetdiagers4,
		uint8(10): cliFlags.Inetdiagers6,
	}

	var inetdiagerStatsLoops int
	var afInetdiagerStatsLoops = map[uint8]int{
		uint8(2):  0,
		uint8(10): 0,
	}
	for inetdiagerStatsWrapper := range in {

		inetdiagerStatsLoops++
		afInetdiagerStatsLoops[inetdiagerStatsWrapper.Af]++

		inetdiagerStaterMsg.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Inc()

		if debugLevel > 100 {
			fmt.Println("inetdiagerStater Af:", inetdiagerStatsWrapper.Af, "\tID:", inetdiagerStatsWrapper.ID, "\tinetdiagerStatsLoops:", inetdiagerStatsLoops, "\tafInetdiagerStatsLoops[inetdiagerStatsWrapper.Af]:", afInetdiagerStatsLoops[inetdiagerStatsWrapper.Af], "\tin:", inetdiagerStatsWrapper)
		}

		oldStats := oldStatsMap[inetdiagerStatsWrapper.Af][inetdiagerStatsWrapper.ID]
		//oldStats, ok := oldStatsMap[inetdiagerStatsWrapper.Af][inetdiagerStatsWrapper.ID]
		// if !ok {
		// 	if debugLevel > 10 {
		// 		fmt.Println("inetdiagerStater AF:", inetdiagerStatsWrapper.Af, "\tID:", inetdiagerStatsWrapper.ID, "\tInitializing")
		// 	}
		// }

		// Calculate differences
		diffStats.InetdiagMsgInSizeTotal = inetdiagerStatsWrapper.Stats.InetdiagMsgInSizeTotal - oldStats.InetdiagMsgInSizeTotal
		diffStats.InetdiagMsgCount = inetdiagerStatsWrapper.Stats.InetdiagMsgCount - oldStats.InetdiagMsgCount
		diffStats.InetdiagMsgFilterCount = inetdiagerStatsWrapper.Stats.InetdiagMsgFilterCount - oldStats.InetdiagMsgFilterCount
		diffStats.InetdiagMsgBytesReadTotal = inetdiagerStatsWrapper.Stats.InetdiagMsgBytesReadTotal - oldStats.InetdiagMsgBytesReadTotal
		diffStats.PadBufferTotal = inetdiagerStatsWrapper.Stats.PadBufferTotal - oldStats.PadBufferTotal
		diffStats.UDPWritesTotal = inetdiagerStatsWrapper.Stats.UDPWritesTotal - oldStats.UDPWritesTotal
		diffStats.UDPBytesWrittenTotal = inetdiagerStatsWrapper.Stats.UDPBytesWrittenTotal - oldStats.UDPBytesWrittenTotal
		diffStats.UDPErrorsTotal = inetdiagerStatsWrapper.Stats.UDPErrorsTotal - oldStats.UDPErrorsTotal
		diffStats.StatsBlocked = inetdiagerStatsWrapper.Stats.StatsBlocked - oldStats.StatsBlocked

		if debugLevel > 100 {
			fmt.Println("inetdiagerStater AF:", kernelEnumToString[inetdiagerStatsWrapper.Af], "\tID:", inetdiagerStatsWrapper.ID, "\tstats:\t\t", inetdiagerStatsWrapper.Stats)
			fmt.Println("inetdiagerStater AF:", kernelEnumToString[inetdiagerStatsWrapper.Af], "\tID:", inetdiagerStatsWrapper.ID, "\toldStats:\t", oldStats)
			fmt.Println("inetdiagerStater AF:", kernelEnumToString[inetdiagerStatsWrapper.Af], "\tID:", inetdiagerStatsWrapper.ID, "\tdiffStats:\t", diffStats)
		}

		inetdiagerIn.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.InetdiagMsgInSizeTotal))
		inetdiagerMsgs.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.InetdiagMsgCount))
		inetdiagerMsgFilters.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.InetdiagMsgFilterCount))
		inetdiagerRead.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.InetdiagMsgBytesReadTotal))
		inetdiagerPad.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.PadBufferTotal))
		inetdiagerUDPs.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.UDPWritesTotal))
		inetdiagerUDPBytes.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.UDPBytesWrittenTotal))
		inetdiagerUDPErrors.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.UDPErrorsTotal))
		inetdiagerStatsBlocked.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af], strconv.FormatInt(int64(inetdiagerStatsWrapper.ID), 10)).Add(float64(diffStats.StatsBlocked))

		// Sum all tyeps of messages for the total
		inetdiagerMsgsTotal.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Add(float64(diffStats.InetdiagMsgCount))
		inetdiagerMsgsTotal.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Add(float64(diffStats.InetdiagMsgFilterCount))
		inetdiagerUDPsTotal.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Add(float64(diffStats.UDPWritesTotal))
		totalInetdiagerMsgs += (diffStats.InetdiagMsgCount + diffStats.InetdiagMsgFilterCount)
		totalInetdiagerUDPs += diffStats.UDPWritesTotal

		// This modulus is a little tricky, cos it's looking up the number of inetdiagers by address family
		// The result is that we only report once for the full set of inetdiager workers
		if inetdiagerStatsLoops%*afToInetdiagers[inetdiagerStatsWrapper.Af] == 0 {
			// Just sending the totals for the moment
			if *cliFlags.NoStatsd == false {
				updateString = fmt.Sprintf("xtcp_%s_inetdiager_total_msgs:%d|g\nxtcp_%s_inetdiager_total_udps:%d|g", kernelEnumToString[inetdiagerStatsWrapper.Af], totalInetdiagerMsgs, kernelEnumToString[inetdiagerStatsWrapper.Af], totalInetdiagerUDPs)
				if debugLevel > 100 {
					fmt.Println("iStater AF:", kernelEnumToString[inetdiagerStatsWrapper.Af], "\tupdateString:\n", updateString)
				}
				udpBytesWritten, udpWriteErr = fmt.Fprintf(udpConn, updateString)
				if udpWriteErr != nil {
					inetdiagerStaterUDPErrors.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Inc()
				}
				inetdiagerStaterUDPs.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Inc()
				inetdiagerStaterUDPBytes.WithLabelValues(kernelEnumToString[inetdiagerStatsWrapper.Af]).Add(float64(udpBytesWritten))
			}
		}

		if *cliFlags.HappyIstaterReportModulus == 1 || afInetdiagerStatsLoops[inetdiagerStatsWrapper.Af]%*cliFlags.HappyIstaterReportModulus == 1 {
			if debugLevel > 10 {
				fmt.Println("iStater afInetdiagerStatsLoops:", afInetdiagerStatsLoops[inetdiagerStatsWrapper.Af], "\tAF:", kernelEnumToString[inetdiagerStatsWrapper.Af], "\ttotalInetdiagerMsgs:\t", diffStats.InetdiagMsgCount, "/", totalInetdiagerMsgs, "\ttotalInetdiagerUDPs:", diffStats.UDPWritesTotal, "/", totalInetdiagerUDPs)
			}
		}

		// store the metrics for next time
		oldStatsMap[inetdiagerStatsWrapper.Af][inetdiagerStatsWrapper.ID] = inetdiagerStatsWrapper.Stats
	}
}
