// Package netlinkerstater is the stats go routine for the xtcp system
//
// Basically, this go routine recieves stats over the channel and updates Prometheus and statsd stats
package netlinkerstater

import (
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"

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

// NetlinkerStatsWrapper struct passes interesting data from the netlinker to the NetlinkerStater
// The wrapper has the af, id, and stats
type NetlinkerStatsWrapper struct {
	Af    uint8
	ID    int
	Stats NetlinkerStats
}

// NetlinkerStats struct are the actual interesting data from each of the Netlinkers
type NetlinkerStats struct {
	PacketsProcessed           int
	NastyContinue              int
	PacketBufferInSizeTotal    int
	NetlinkMsgCountTotal       int
	PacketBufferBytesReadTotal int
	InetdiagMsgCopyBytesTotal  int
	NetlinkMsgErrorCount       int
	OutBlocked                 int
	LongestBlockedDuration     time.Duration
}

// NetlinkerStater is responsible for incrementing prometheus stats and optionally statsd about the netlink workers
// netlinkerStater recieves summary stats from each netlinker worker just as it completes
// Reminder that the netlinker complettes when all packets have been processed, and the process has reached DONE or timeout
func NetlinkerStater(in <-chan NetlinkerStatsWrapper, cliFlags cliflags.CliFlags) {

	//TOTO add packetBuffer size as static variable

	//------------------------------
	// netlinker static settings
	netlinkerPacketSize := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "packet_size",
			Help:      "netlinker netlink packetSize (bytes) read from kernel",
		},
	)
	var packetBufferSize int
	if *cliFlags.PacketSize == 0 {
		packetBufferSize = syscall.Getpagesize() * *cliFlags.PacketSizeMply
	} else {
		packetBufferSize = *cliFlags.PacketSize * *cliFlags.PacketSizeMply
	}
	netlinkerPacketSize.Set(float64(packetBufferSize))

	netlinkerSamplingModulus := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "sampling_modulus",
			Help:      "netlinker samplingModulus will sample every X inetdiag messages to send to inetdiager.",
		},
	)
	netlinkerSamplingModulus.Set(float64(*cliFlags.SamplingModulus))

	//------------------------------
	// netlinker
	netlinkerPackets := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "packets",
			Help:      "netlinker packets, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerNasty := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "nasty",
			Help:      "netlinker nasty continues (where we are ignoring errors and just keep going), by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerIn := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "in",
			Help:      "netlinker INput bytes (amount read from the kernel), by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerMsgs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "msgs",
			Help:      "netlinker netlink messages, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerRead := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "read",
			Help:      "netlinker buffer bytes read, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerErrors := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "errors",
			Help:      "netlinker netlink message errors, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerOut := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "out",
			Help:      "netlinker bytes OUT over the channel to inetdiagers, by address family, by worker id",
		},
		[]string{"af", "id"},
	)
	netlinkerBlocked := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "blocked",
			Help:      "netlinker output channel blocked counter (probably indicated channel not large enough, or not enough inetdiagers), by address family, by worker id",
		},
		[]string{"af", "id"},
	)

	// Blocked duration summary (NOT sending to statsd)
	// Please note by "longest",  we mean the longest duration the channel was blocked for each netlinker during the netlinker's lifetime.
	// Therefore please be careful to remember that 50th percentile is NOT the 50th percentile of the blocked duration
	// This is for tracking the longestBlockedDuration for the send channel in the netlinkers to the inetdiagers
	// Warning - Summaries are relatiely expensive
	// See also: https://prometheus.io/docs/practices/histograms/
	// https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts
	var netlinkerBlockedSum = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "xtcp",
			Subsystem: "netlinker",
			Name:      "longest_blocked_duration_summary",
			Help:      "netlinker LONGEST duration blocked on the output channel summary, by address family",
			Objectives: map[float64]float64{ // 50th, 99th
				0.5:  0.05,
				0.99: 0.001},
			MaxAge: 5 * time.Minute, // 5 minutes of data
		},
		[]string{"af"},
	)

	//-------------------
	// netlinkerStater prometheus counters
	// Please note, these are NOT being sent to statsd
	netlinkerStaterMsg := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinkerStater",
			Name:      "msgs",
			Help:      "netlinkerStater messages recieved on the channel, by address family, by id",
		},
		[]string{"af"},
	)
	netlinkerStaterUDPs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinkerStater",
			Name:      "udps",
			Help:      "netlinkerStater UDP messages sent (likely to be packets), by address family, by id",
		},
		[]string{"af"},
	)
	netlinkerStaterUDPBytes := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinkerStater",
			Name:      "udp_bytes",
			Help:      "netlinkerStater UDP bytes sent, by address family, by id",
		},
		[]string{"af"},
	)
	netlinkerStaterUDPErrors := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "netlinkerStater",
			Name:      "udp_errors",
			Help:      "netlinkerStater UDP messages send errors, by address family, by id",
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
	if !*cliFlags.NoStatsd {
		udpConn, dialerr = net.Dial("udp", *cliFlags.StatsdDst)
		if dialerr != nil {
			if debugLevel > 10 {
				fmt.Println("pollerStater:\tnet.Dial(\"udp\", ", *cliFlags.StatsdDst, ") error:", dialerr)
			}
		}
		defer udpConn.Close()
	}

	for netlinkerStatsWrapper := range in {

		netlinkerStaterMsg.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()

		if debugLevel > 100 {
			fmt.Println("netlinkerStats aA:", netlinkerStatsWrapper.Af, "\tID:", netlinkerStatsWrapper.ID, "\tin\t\t:", netlinkerStatsWrapper)
		}

		netlinkerPackets.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.PacketsProcessed))
		netlinkerNasty.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.NastyContinue))
		netlinkerIn.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.PacketBufferInSizeTotal))
		netlinkerMsgs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.NetlinkMsgCountTotal))
		netlinkerRead.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.PacketBufferBytesReadTotal))
		netlinkerErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.NetlinkMsgErrorCount))
		netlinkerOut.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.InetdiagMsgCopyBytesTotal))
		netlinkerBlocked.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10)).Add(float64(netlinkerStatsWrapper.Stats.OutBlocked))
		netlinkerBlockedSum.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Observe(netlinkerStatsWrapper.Stats.LongestBlockedDuration.Seconds())

		if debugLevel > 100 {
			fmt.Println("netlinkerStats Af:", netlinkerStatsWrapper.Af, "\tID:", netlinkerStatsWrapper.ID, "\tOutBlocked:", netlinkerStatsWrapper.Stats.OutBlocked, "\tLongestBlockedDuration:", netlinkerStatsWrapper.Stats.LongestBlockedDuration.Seconds())
		}

		// TODO could potentially move the UDP sending to a different work to allow inserting of some sleeps to not overwhealm stats (should be ok given the rate is now low)
		if !*cliFlags.NoStatsd {

			// packets
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_packets:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.PacketsProcessed))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// nasty
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_nasty:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.NastyContinue))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// in
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_in:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.PacketBufferInSizeTotal))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// msgs
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_msgs:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.NetlinkMsgCountTotal))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// read
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_read:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.PacketBufferBytesReadTotal))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// out
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_out:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.InetdiagMsgCopyBytesTotal))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			// errors
			updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_errors:%d|g", kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10), int(netlinkerStatsWrapper.Stats.NetlinkMsgErrorCount))
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			}
			netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
			netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))

			if netlinkerStatsWrapper.Stats.OutBlocked > 0 {
				// blocked and longest blocked duration
				updateString = fmt.Sprintf("xtcp_%s_netlinker_%s_blocked:%d|g\nxtcp_%s_netlinker_%s_longest_blocked_duration:%f|g",
					kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10),
					int(netlinkerStatsWrapper.Stats.OutBlocked), kernelEnumToString[netlinkerStatsWrapper.Af], strconv.FormatInt(int64(netlinkerStatsWrapper.ID), 10),
					netlinkerStatsWrapper.Stats.LongestBlockedDuration.Seconds())
				if debugLevel > 100 {
					fmt.Println("netlinkerStats Af:", netlinkerStatsWrapper.Af, "\tupdateString:", updateString)
				}
				udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
				if udpWriteErr != nil {
					netlinkerStaterUDPErrors.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
				}
				netlinkerStaterUDPs.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Inc()
				netlinkerStaterUDPBytes.WithLabelValues(kernelEnumToString[netlinkerStatsWrapper.Af]).Add(float64(udpBytesWritten))
			}
		}
	}
}
