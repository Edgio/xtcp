package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/disabler"
	"github.com/Edgio/xtcp/pkg/inetdiagerstater"
	"github.com/Edgio/xtcp/pkg/misc"
	"github.com/Edgio/xtcp/pkg/netlinkerstater"
	"github.com/Edgio/xtcp/pkg/poller"
	"github.com/Edgio/xtcp/pkg/pollerstater"
	"github.com/Edgio/xtcp/pkg/xtcpstater"
	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sys/unix"
)

const (
	debugLevel int = 11
)

var (
	// Passed by "go build -ldflags" for the show version
	commit string
	date   string
)

// main function is responsible for a few key activities
// 0. Exits if we aren't running on Linux
// 1. Handles all the CLI flags
// 1.1 Populates a big cliFlags struct to make it easy to pass to other goroutines
// 3. Version printing
// 4. Starts disablement checker (disabler) goroutine
// 5. Allows for profiling options
// 6. Starts the staters (the multiple metrics go routines), which includes the Prometheus metric endpoints HTTP handler
// 7. Starts the poller which is really the main loop for xtcp
func main() {

	misc.DieIfNotLinux()

	no4 := flag.Bool("no4", false, "no IPv4, default false = IPv4 enabled")
	no6 := flag.Bool("no6", false, "no IPv6, default false = IPv6 enabled")
	timeout := flag.Int64("timeout", 50, "Netlink socket timeout in milliseconds.  Zero(0) for no timeout.  Default 50 ms") // can be increased to 100ms if needed
	pollingFrequency := flag.Duration("frequency", 30*time.Second, "Polling frequency. Default 30 seconds")                 // TODO make default 10s
	pollingSafetyBuffer := flag.Float64("pollingSafetyBuffer", 0.8, "pollingSafetyBuffer defines the point at which warnings about long polling duration are generated, defined as a percentage of pollingFrequencySeconds.  Default = 0.8 (80%)")
	maxLoops := flag.Int("maxLoops", 0, "Maximum number of loops, or zero (0) for forever.  Default 0")
	// Option to allow shutting down workers between polls
	shutdownWorkers := flag.Bool("shutdownWorkers", false, "shutdownWorkers, default false")
	// Numbers of workers
	netlinkers4 := flag.Int("netlinkers4", 4, "netlinkers4, default 4")      //4
	netlinkers6 := flag.Int("netlinkers6", 2, "netlinkers6, default 2")      //2
	inetdiagers4 := flag.Int("inetdiagers4", 10, "inetdiagers4, default 10") //10
	inetdiagers6 := flag.Int("inetdiagers6", 4, "inetdiagers6, default 4")   //4
	// Shortcut for single (x1) worker of each type (which helps debug with less concurrency)
	single := flag.Bool("single", false, "Single means only one (1) of each worker type")
	nlmsgSeq := flag.Int("nlmsgSeq", 666, "nlmsgSeq sequence number (start), which should be uint32")
	// packetSize of the buffer the netlinkers syscall.Recvfrom to read into
	// TODO profile changing this.  Quick test of 2048 and 8192 shows changing this works fine
	packetSize := flag.Int("packetSize", 0, "netlinker packetSize.  buffer size = packetSize * packetSizeMply. Use zero (0) for syscall.Getpagesize(). Default = 0")
	packetSizeMply := flag.Int("packetSizeMply", 8, "netlinker packetSize multiplier.  buffer size = packetSize * packetSizeMply.  Default = 8")

	// Channel sizes
	netlinkerChSize := flag.Int("netlinkerChSize", 100, "netlinkerChSize, Default 100")

	// Control how many messages to from netlink to inetdiager
	samplingModulus := flag.Int("samplingModulus", 2, "samplingModulus.  Netlinker will sample every Xth inetdiag messages to send to inetdiager. Default 2") //TODO make default 1
	// CLI standard out reporting modulus.  e.g. report every x inetd messages
	inetdiagerReportModulus := flag.Int("inetdiagerReportModulus", 2000, "inetdiagerReportModulus. Report every X inetd messages to Kafka. Default 2000") //TODO make default 1000
	inetdiagerFilterReportModulus := flag.Int("inetdiagerFilterReportModulus", 2000, "inetdiagerFilterReportModulus. Report every X inetd messages that matches the filter to Kafka. Default 2000")

	inetdiagerStatsRatio := flag.Float64("inetdiagerStatsRatio", 0.9, "inetdiagerStatsRatio controls the how often the inetdiagers send summary stats, which is as a percentage of the pollingFrequencySeconds. Default = 0.9 (90% of pollingFrequencySeconds)")

	// UDP send destination
	udpSendDest := flag.String("udpSendDest", "127.0.0.1:13000", "UDP socket send destination. Default = 127.0.0.1:13000")

	// Prometheus related
	//promListen := flag.String("promListen", "[::1]:9000", "Prometheus http listening socket. Use 0.0.0.0:9000 for all interfaces. Default = [::1]:9000")
	promListen := flag.String("promListen", "127.0.0.1:9000", "Prometheus http listening socket. Use 0.0.0.0:9000 for all interfaces. Default = 127.0.0.1:9000")
	promPath := flag.String("promPath", "/metrics", "Prometheus http path. Default = /metrics")
	// curl -s http://[::1]:9000/metrics 2>&1 | grep -v "#"
	// curl -s http://127.0.0.1:9000/metrics 2>&1 | grep -v "#"

	// We do NOT want inetdiagers to block when sending these stats over the channel.
	// It's OK if they do backup in the channel, because we should have plenty of idle time at the end of the polling loop to catch up on this work
	promPollerChSize := flag.Int("promPollerChSize", 4, "promPollerChSize is the channel size for the pollerStaterCh, Default 4")
	promNetlinkerChSize := flag.Int("promNetlinkerChSize", 10, "promChSize is the channel size for the netlinkerStaterCh, Default 10")
	promInetdiagerChSize := flag.Int("promInetdiagerChSize", 100, "promChSize is the channel size for the inetdiagerStaterCh, Default 100")
	// TODO we could potentially have multiple counterIncrementer workers

	// Statsd related
	statsdDst := flag.String("statdDst", "127.0.0.1:8125", "Statds UDP socket destination. Default = 127.0.0.1:8125")
	noStatsd := flag.Bool("noStatsd", false, "no noStatsd, default false = noStatsd enabled")
	// tcpdump -ni lo -vv -X udp port 8125
	// # cat statsd.conf
	// LoadPlugin statsd

	// <Plugin statsd>
	//   Host "127.0.0.1"
	//   Port 8125
	//   DeleteTimers true
	//   DeleteSets true
	//   DeleteCounters false
	//   DeleteGauges true
	// </Plugin>

	// Go runtime & profiling
	// Maximum number of CPUs that can be executing simultaneously
	// https://golang.org/pkg/runtime/#GOMAXPROCS -> zero (0) means default
	goMaxProcs := flag.Int("goMaxProcs", 4, "goMaxProcs = https://golang.org/pkg/runtime/#GOMAXPROCS, default = 4. 0 = golang default.")
	profileMode := flag.String("profile.mode", "", "enable profiling mode, one of [cpu, mem, mutex, block]")

	// Happy noise
	happyPollerReportModulus := flag.Int("happyPollerReportModulus", 1000, "xtcp poller emits some non-error/happy log messages, and this modules controls the rate. Default = 10")
	happyIstaterReportModulus := flag.Int("happyIstaterReportModulus", 10000, "xtcp inetdiagstater emits some non-error/happy log messages, and this modules controls the rate. Default = 1000")

	// Variables relating to the "disabler" go routine which polls for if xtcp should be running
	noDisabler := flag.Bool("noDisabler", false, "Flag to disable the Disabler poller, default false = Disabler enabled")
	disablerFrequency := flag.Duration("disablerFrequency", 60*time.Second, "Disabler polling frequency. Default 60 seconds")
	disablerCommand := flag.String("disablerCommand", "echo", "Command to run/poll to check if xtcp should exit(0)")
	disablerArgument1 := flag.String("disablerArgument1", "$XTCP_DISABLED", "Argument to disablerCommand")

	// XTCP stater
	xTCPStaterFrequency := flag.Duration("xTCPStaterFrequencySeconds", 60*time.Second, "XTCP stater reporting frequency. Default 60 seconds")
	xTCPStaterSystemctlPath := flag.String("xTCPStaterSystemctlPath", "/bin/systemctl", "Full system path to systemctl.  Default \"/bin/systemctl\"")
	xTCPStaterPsPath := flag.String("xTCPStaterPsPath", "/bin/ps", "Full system path to ps.  Default \"/bin/ps\"")
	// Controls to include or disclude loopbacks socks
	includeLoopback := flag.Bool("includeLoopback", false, "Include loopback in collection. Default: false")

	// Controls for the pop local block filters
	enableFilter := flag.Bool("enableFilter", false, "Subsample sockets that match the filter blocks. Default: false")
	filterJson := flag.String("filterJson", "", "Json definition of the filter groups.")
	filterGroup := flag.String("filterGroup", "", "Name of filter group used in top level of filterJson.")

	version := flag.Bool("version", false, "show version")
	defaults := flag.Bool("defaults", false, "show default configuration")

	nsq := flag.String("nsq", "", "Write to NSQ IP:Port")

	flag.Parse()

	// Print version information passed in via ldflags in the Makefile
	if *version {
		//log.Fatalf("xtcp commit:%s\tdate:%s", commit, date)
		fmt.Println("xtcp commit:", commit, "\tdate(UTC):", date)
		os.Exit(0)
	}

	// Print out defaults
	if *defaults {
		if debugLevel > 10 {
			fmt.Println("*no4:", *no4)
			fmt.Println("*no6:", *no6)
			fmt.Println("*timeout:", *timeout, "(ms)")
			fmt.Println("*pollingFrequency:", *pollingFrequency)
			fmt.Println("*pollingSafetyBuffer:", *pollingSafetyBuffer)
			fmt.Println("*maxLoops:", *maxLoops)
			fmt.Println("*shutdownWorkers:", *shutdownWorkers)
			fmt.Println("*netlinkers4:", *netlinkers4)
			fmt.Println("*netlinkers6:", *netlinkers6)
			fmt.Println("*inetdiagers4:", *inetdiagers4)
			fmt.Println("*inetdiagers6:", *inetdiagers6)
			fmt.Println("*single:", *single)
			fmt.Println("*nlmsgSeq:", *nlmsgSeq)
			fmt.Println("*packetSize:", *packetSize)
			fmt.Println("*packetSizeMply:", *packetSizeMply)
			fmt.Println("*netlinkerChSize:", *netlinkerChSize)
			fmt.Println("*samplingModulus:", *samplingModulus)
			fmt.Println("*inetdiagerReportModulus:", *inetdiagerReportModulus)
			fmt.Println("*inetdiagerFilterReportModulus:", *inetdiagerFilterReportModulus)
			fmt.Println("*inetdiagerStatsRatio:", *inetdiagerStatsRatio)
			fmt.Println("*udpSendDest:", *udpSendDest)
			fmt.Println("*promListen:", *promListen)
			fmt.Println("*promPath:", *promPath)
			fmt.Println("*promPollerChSize:", *promPollerChSize)
			fmt.Println("*promNetlinkerChSize:", *promNetlinkerChSize)
			fmt.Println("*promInetdiagerChSize:", *promInetdiagerChSize)
			fmt.Println("*statsdDst:", *statsdDst)
			fmt.Println("*noStatsd:", *noStatsd)
			fmt.Println("*goMaxProcs:", *goMaxProcs)
			fmt.Println("*happyPollerReportModulus:", *happyPollerReportModulus)
			fmt.Println("*happyIstaterReportModulus:", *happyIstaterReportModulus)
			fmt.Println("*noDisabler:", *noDisabler)
			fmt.Println("*disablerFrequency:", *disablerFrequency)
			fmt.Println("*disablerCommand:", *disablerCommand)
			fmt.Println("*disablerArgument1:", *disablerArgument1)
			fmt.Println("*xTCPStaterFrequency:", *xTCPStaterFrequency)
			fmt.Println("*xTCPStaterSystemctlPath:", *xTCPStaterSystemctlPath)
			fmt.Println("*xTCPStaterPsPath:", *xTCPStaterPsPath)
			fmt.Println("*includeLoopback:", *includeLoopback)
			fmt.Println("*enableFilter:", *enableFilter)
			fmt.Println("*filterJson:", *filterJson)
			fmt.Println("*filterGroup:", *filterGroup)
			fmt.Println("*nsq:", *nsq)
		}
		os.Exit(0)
	}

	if *single == true {
		*netlinkers4 = 1
		*netlinkers6 = 1
		*inetdiagers4 = 1
		*inetdiagers6 = 1
	}

	if debugLevel > 100 {
		fmt.Println("*netlinkers4:", *netlinkers4)
		fmt.Println("*netlinkers6:", *netlinkers6)
		fmt.Println("*inetdiagers4:", *inetdiagers4)
		fmt.Println("*inetdiagers6:", *inetdiagers6)
	}

	// Copy all the CLI flags to the struct, to make passing the flags more simple
	var cliFlags cliflags.CliFlags
	cliFlags.No4 = no4
	cliFlags.No6 = no6
	cliFlags.Timeout = timeout
	cliFlags.PollingFrequency = pollingFrequency
	cliFlags.PollingSafetyBuffer = pollingSafetyBuffer
	cliFlags.MaxLoops = maxLoops
	cliFlags.ShutdownWorkers = shutdownWorkers
	cliFlags.Netlinkers4 = netlinkers4
	cliFlags.Netlinkers6 = netlinkers6
	cliFlags.Inetdiagers4 = inetdiagers4
	cliFlags.Inetdiagers6 = inetdiagers6
	cliFlags.Single = single
	cliFlags.NlmsgSeq = nlmsgSeq
	cliFlags.PacketSize = packetSize
	cliFlags.PacketSizeMply = packetSizeMply
	cliFlags.NetlinkerChSize = netlinkerChSize
	cliFlags.SamplingModulus = samplingModulus
	cliFlags.InetdiagerReportModulus = inetdiagerReportModulus
	cliFlags.InetdiagerFilterReportModulus = inetdiagerFilterReportModulus
	cliFlags.InetdiagerStatsRatio = inetdiagerStatsRatio
	cliFlags.GoMaxProcs = goMaxProcs
	cliFlags.UDPSendDest = udpSendDest
	cliFlags.PromListen = promListen
	cliFlags.PromPath = promPath
	cliFlags.PromPollerChSize = promPollerChSize
	cliFlags.PromNetlinkerChSize = promNetlinkerChSize
	cliFlags.PromInetdiagerChSize = promInetdiagerChSize
	cliFlags.StatsdDst = statsdDst
	cliFlags.NoStatsd = noStatsd
	cliFlags.HappyPollerReportModulus = happyPollerReportModulus
	cliFlags.HappyIstaterReportModulus = happyIstaterReportModulus
	cliFlags.NoDisabler = noDisabler
	cliFlags.DisablerFrequency = disablerFrequency
	cliFlags.DisablerCommand = disablerCommand
	cliFlags.DisablerArgument1 = disablerArgument1
	cliFlags.XTCPStaterFrequency = xTCPStaterFrequency
	cliFlags.XTCPStaterSystemctlPath = xTCPStaterSystemctlPath
	cliFlags.XTCPStaterPsPath = xTCPStaterPsPath
	cliFlags.IncludeLoopback = includeLoopback
	cliFlags.EnableFilter = enableFilter
	cliFlags.FilterJson = filterJson
	cliFlags.FilterGroup = filterGroup
	cliFlags.NSQ = nsq

	// Start background polling job to cleanly exit if the return code of executing 'disablerCommand' is "1"
	// Using a channel here to block waiting for disabler.Disabler to complete once before proceeding passed this main block
	// Otherwise, golang is so fast that it races ahead and actually starts polling etc below before this check completes
	var disablerCheckComplete chan struct{}
	disablerCheckComplete = make(chan struct{}, 2)
	if *cliFlags.NoDisabler == false {
		go disabler.Disabler(cliFlags, disablerCheckComplete, false)
		// block waiting for disabler on the first iteration
		<-disablerCheckComplete
		if debugLevel > 10 {
			fmt.Println("Disable check complete")
		}
	}

	mp := runtime.GOMAXPROCS(*cliFlags.GoMaxProcs)
	if debugLevel > 10 {
		fmt.Println("Main runtime.GOMAXPROCS was:", mp)
	}

	// profiling goodness
	// "github.com/pkg/profile"
	// https://dave.cheney.net/2013/07/07/introducing-profile-super-simple-profiling-for-go-programs
	// e.g. ./xtcp -profile.mode trace
	// go tool trace trace.out
	// e.g. ./xtcp -profile.mode cpu
	// go tool pprof -http=":8081" xtcp cpu.pprof

	if debugLevel > 10 {
		fmt.Println("*profileMode:", *profileMode)
	}
	switch *profileMode {
	case "cpu":
		defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	case "mem":
		// this is the heap profile
		defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()
	// case "alloc":
	// 	defer profile.Start(profile.MemProfileAllocs, profile.ProfilePath(".")).Stop()
	case "mutex":
		defer profile.Start(profile.MutexProfile, profile.ProfilePath(".")).Stop()
	case "block":
		defer profile.Start(profile.BlockProfile, profile.ProfilePath(".")).Stop()
	case "trace":
		defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
	default:
		if debugLevel > 10 {
			fmt.Println("No profiling")
		}
	}

	// Start prometheus exporter
	//http.Handle("/metrics", promhttp.Handler())
	// https: //pkg.go.dev/github.com/prometheus/client_golang/prometheus/promhttp?tab=doc#HandlerOpts
	http.Handle(*promPath, promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))
	go http.ListenAndServe(*promListen, nil)
	if debugLevel > 10 {
		fmt.Println("Prometheus http listener started on:", *promListen, *promPath)
	}
	// Start the stats workers
	// Please note theres a single (x1) worker of each type currently,
	// because making the prometheus counters concurrently is a little tricky
	// Potentially, you could just register all the prometheus in an init function
	// and then pass those pointers to all the workers, and just call directly in the code
	// (this might actually be the recommended way)
	// TODO. Improve concurrency

	// xtcpstater reports on the over all xtcp process, via "systemctl status" and "ps"
	go xtcpstater.XTCPStater(cliFlags)

	var pollerStaterCh chan pollerstater.PollerStats
	pollerStaterCh = make(chan pollerstater.PollerStats, *cliFlags.PromPollerChSize)
	go pollerstater.PollerStater(pollerStaterCh, cliFlags)

	var netlinkerStaterCh chan netlinkerstater.NetlinkerStatsWrapper
	netlinkerStaterCh = make(chan netlinkerstater.NetlinkerStatsWrapper, *cliFlags.PromNetlinkerChSize)
	go netlinkerstater.NetlinkerStater(netlinkerStaterCh, cliFlags)

	var inetdiagerStaterCh chan inetdiagerstater.InetdiagerStatsWrapper
	inetdiagerStaterCh = make(chan inetdiagerstater.InetdiagerStatsWrapper, *cliFlags.PromInetdiagerChSize)
	go inetdiagerstater.InetdiagerStater(inetdiagerStaterCh, cliFlags)

	if debugLevel > 10 {
		fmt.Println("Main staters started")
	}

	// Hostname is required for the XtcpRecord.hostname proto, and called here once rather than per address family in the pollers
	var hostname string
	hostname = misc.GetHostname()
	if debugLevel > 10 {
		fmt.Println("Main hostname:", hostname)
	}

	// Setup addressFamilies to iterate over
	// We're doing most things for both IPv4 and IPv6
	var addressFamilies []uint8
	if !*no4 {
		addressFamilies = append(addressFamilies, unix.AF_INET)
	}
	if !*no6 {
		addressFamilies = append(addressFamilies, unix.AF_INET6)
	}

	// Start poller per address family
	var pollerWG sync.WaitGroup
	for _, addressFamily := range addressFamilies {
		if debugLevel > 10 {
			fmt.Println("Main starting poller:", addressFamily, "(", misc.KernelEnumToString[addressFamily], ")")
		}
		pollerWG.Add(1)
		go poller.Poller(addressFamily, &hostname, cliFlags, &pollerWG, pollerStaterCh, netlinkerStaterCh, inetdiagerStaterCh)
	}
	pollerWG.Wait()

	if debugLevel > 10 {
		fmt.Println("Main done")
	}
	return
}
