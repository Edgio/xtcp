// Package pollerstater is the stats handling go routine of xtcp
//
// Basically, this go routine recieves stats over the channel and updates Prometheus and statsd stats
package pollerstater

import (
	"fmt"
	"net"
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

// PollerStats struct has simple stats about the poller goroutine
type PollerStats struct {
	Af                 uint8
	PollingLoops       int
	PollToDoneDuration time.Duration
	PollDuration       time.Duration
}

// PollerStater calculates stats for the pollers
// We're calculating the differences between stats, for multiple reasons
// 1. Difference means we increment the prometheus counters less frequently and in bigger chunks
// 2. We can send the diffs to statsd to avoid overloading it with lots of small increments
// 3. We only need to pass things over the channel relatively infrequently
// cliFlags needed for statsd destination
func PollerStater(in <-chan PollerStats, cliFlags cliflags.CliFlags) {

	//---------------------------------
	// Register Prometheus metrics

	//-------------------
	// Register static variables

	// Poller frequency gauge
	// This is the poller frequency.  Although this is based on CLI flags, including
	// this in Prometheus, so we can setup an alarm.  e.g. If poll duration gets to 80% of poller frequency
	promFrequencyGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "frequency",
			Help:      "poller frequency",
		},
	)
	promFrequencyGauge.Set((*cliFlags.PollingFrequency).Seconds())

	// max loops
	promMaxLoopsGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "max_loops",
			Help:      "poller maximum loops (0 = forever)",
		},
	)
	promMaxLoopsGauge.Set(float64(*cliFlags.MaxLoops))

	//-------------------
	// Register the non-static variables

	// Durations summary
	// This is for tracking "done" and "poll" durations
	// Warning - Summaries are relatiely expensive - doing this because the durations vary quite a lot, so will give us a nice view
	// To ensure this doesn't use too many resources, it's limited to 5 minutes.  (5 minutes / poll_frequency = samples to be sorted)
	// See also: https://prometheus.io/docs/practices/histograms/
	// https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts
	var promDurationSumVec = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "duration_summary",
			Help:      "poller duration summary, by address family, and duration type",
			Objectives: map[float64]float64{ // 25th, 50th, 75th, 90th, 99th
				0.25: 0.05,
				0.5:  0.05,
				0.75: 0.05,
				0.9:  0.01,
				0.99: 0.001},
			MaxAge: 5 * time.Minute, // 5 minutes of data
		},
		[]string{"af", "type"},
	)

	// if err := prometheus.Register(promDurationSumVec); err != nil {
	// 	if debugLevel > 10 {
	// 		fmt.Println("promDurationSumVec not registered:", err)
	// 	}
	// } else {
	// 	if debugLevel > 10 {
	// 		fmt.Println("promDurationSumVec registered.")
	// 	}
	// }

	// Poller duration guages
	// This is for a guage for duration of "done" and "poll"
	// Guages are recommended to be histograms and/or summaries, but we're adding this just for ease of use
	promDurationGaugeVec := promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "duration",
			Help:      "poller duration guage, by address family, and duration type (done/poll)",
		},
		[]string{"af", "type"},
	)

	pollingLoops := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "loops",
			Help:      "poller loops, by address family",
		},
		[]string{"af"},
	)

	pollingLong := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "poller",
			Name:      "long_poll",
			Help:      "poller number of times the polling pool has taken longer than pollingSafetyBuffer %, by address family",
		},
		[]string{"af"},
	)

	//-------------------
	// pollerStater prometheus counters
	// Please note, these are NOT being sent to statsd
	pollerStaterMsgs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "pollerStater",
			Name:      "msgs",
			Help:      "pollerStater messages recieved on the channel, by address family",
		},
		[]string{"af"},
	)
	pollerStaterUDPs := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "pollerStater",
			Name:      "udps",
			Help:      "pollerStater UDP messages sent (likely to be packets), by address family",
		},
		[]string{"af"},
	)

	pollerStaterUDPBytes := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "pollerStater",
			Name:      "udp_bytes",
			Help:      "pollerStater UDP bytes sent, by address family",
		},
		[]string{"af"},
	)

	pollerStaterUDPErrors := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "pollerStater",
			Name:      "udp_errors",
			Help:      "pollerStater UDP messages send errors, by address family",
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

	oldStatsMap := make(map[uint8]PollerStats)
	var diffStats PollerStats

	for pollerStats := range in {

		pollerStaterMsgs.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()

		promDurationSumVec.WithLabelValues(kernelEnumToString[pollerStats.Af], "done").Observe(pollerStats.PollToDoneDuration.Seconds())
		promDurationGaugeVec.WithLabelValues(kernelEnumToString[pollerStats.Af], "done").Set(pollerStats.PollToDoneDuration.Seconds())
		promDurationSumVec.WithLabelValues(kernelEnumToString[pollerStats.Af], "poll").Observe(pollerStats.PollDuration.Seconds())
		promDurationGaugeVec.WithLabelValues(kernelEnumToString[pollerStats.Af], "poll").Set(pollerStats.PollDuration.Seconds())

		// Calculate differences
		diffStats.PollingLoops = pollerStats.PollingLoops - oldStatsMap[pollerStats.Af].PollingLoops
		//diffStats.pollToDoneDuration = pollerStats.pollToDoneDuration - oldStatsMap[pollerStats.Af].pollToDoneDuration
		//diffStats.pollDuration = pollerStats.pollDuration - oldStatsMap[pollerStats.Af].pollDuration
		oldStatsMap[pollerStats.Af] = pollerStats

		pollingLoops.WithLabelValues(kernelEnumToString[pollerStats.Af]).Add(float64(diffStats.PollingLoops))

		if debugLevel > 100 {
			fmt.Println("pollerStater Af:", pollerStats.Af, "\tdiffStats.PollingLoops:", diffStats.PollingLoops)
			//fmt.Println("pollerStater Af:", pollerStats.Af, "\tdiffStats.pollToDoneDuration.Seconds():", diffStats.pollToDoneDuration.Seconds())
			//fmt.Println("pollerStater Af:", pollerStats.Af, "\tdiffStats.pollDuration.Seconds():", diffStats.pollDuration.Seconds())
		}

		// TODO could potentially move the UDP sending to a different work to allow inserting of some sleeps to not overwhealm stats (should be ok given the rate is now low)
		if !*cliFlags.NoStatsd {

			// pollingLoops
			updateString = fmt.Sprintf("xtcp_%s_poller_loops:%d|g", kernelEnumToString[pollerStats.Af], int(pollerStats.PollingLoops))
			if debugLevel > 100 {
				fmt.Println("pollerStater Af:", pollerStats.Af, "\tupdateString:", updateString)
			}
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				pollerStaterUDPErrors.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
			}
			pollerStaterUDPs.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
			pollerStaterUDPBytes.WithLabelValues(kernelEnumToString[pollerStats.Af]).Add(float64(udpBytesWritten))

			// pollToDoneDuration & pollDuration
			// Please note that statsd doco says it support milliseconds ( https://github.com/statsd/statsd/blob/master/docs/metric_types.md )
			// Please further note that the statsd doco is incorrect, and our collectd stats supports seconds only: https://github.com/collectd/collectd/blob/main/src/statsd.c#L96
			updateString = fmt.Sprintf("xtcp_%s_done_duration:%f|g\nxtcp_%s_poll_duration:%f|g", kernelEnumToString[pollerStats.Af], pollerStats.PollToDoneDuration.Seconds(), kernelEnumToString[pollerStats.Af], pollerStats.PollDuration.Seconds())
			if debugLevel > 100 {
				fmt.Println("pollerStater Af:", pollerStats.Af, "\tupdateString:", updateString)
			}
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				pollerStaterUDPErrors.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
			}
			pollerStaterUDPs.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
			pollerStaterUDPBytes.WithLabelValues(kernelEnumToString[pollerStats.Af]).Add(float64(udpBytesWritten))
		}

		// If the polling loop is taking to long, increase the long poll counter
		if pollerStats.PollDuration > (time.Duration(float64(*cliFlags.PollingFrequency) * *cliFlags.PollingSafetyBuffer)) {
			if debugLevel > 100 {
				fmt.Println("pollerStater Af:", pollerStats.Af, "\tPOLLING IS TAKING TOO LONG!! WARNING!!")
			}
			pollingLong.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()

			if !*cliFlags.NoStatsd {

				// polling Long
				updateString = fmt.Sprintf("xtcp_%s_poller_long:%d|c", kernelEnumToString[pollerStats.Af], int(1))
				udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
				if udpWriteErr != nil {
					pollerStaterUDPErrors.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
				}
				pollerStaterUDPs.WithLabelValues(kernelEnumToString[pollerStats.Af]).Inc()
				pollerStaterUDPBytes.WithLabelValues(kernelEnumToString[pollerStats.Af]).Add(float64(udpBytesWritten))
			}
		}
	}
}
