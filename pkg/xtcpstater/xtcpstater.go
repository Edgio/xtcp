// Package xtcpstater creates promeheus and statsd stats about the overall xtcp progess
//
// The prometheus client automagically provides a bunch of useful one, but this isn't true for statsd
//
// The main metric this package provides are the stats from the systemd "systemctl status xtcp"
// - tasks
// - memory usage
//
// Trying to do this WIHTOUT regexes if possible

// TODO Not shell out to "ps" for memory stats, and instead use the go runtime https://golang.org/pkg/runtime/#MemStats

// TODO It is apparently poor form to not know how this routine will close cleanly.  Fix this. e.g. Add a channel or whatever to allow signaling

// os.Getppid()

package xtcpstater

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/misc"
	"github.com/go-cmd/cmd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	debugLevel int = 11
)

// ParseGetSystemCtlStatus parses the output of "systemctl status xtcp.service"
func ParseGetSystemCtlStatus(lines []string) (pid string, tasks string) {

	// Hard coded map of which lines and which columns we want to extract
	//-----
	//  Main PID: 11879 (xtcp)                        <--- PID
	//     Tasks: 12 (limit: 32)                               <------ TASKS
	//-----
	interestingAttributes := [...]string{"PID", "Tasks"}
	var interestingColumnMap = map[string]int{
		"PID":   2,
		"Tasks": 1,
	}

	for i, line := range lines {
		if debugLevel > 100 {
			fmt.Println("i:", i, "\tline:", line)
		}
		for _, attribute := range interestingAttributes {
			if strings.Contains(line, attribute) {
				if debugLevel > 100 {
					fmt.Println("i:", i, "\tfound attribute:", attribute)
				}
				parts := strings.Fields(line)
				for j, part := range parts {
					if debugLevel > 100 {
						fmt.Println("j:", j, "\tpart:", part)
					}
					if j == interestingColumnMap[attribute] {
						if debugLevel > 100 {
							fmt.Println("j:", j, "\tattribute:", attribute, "\tfound column:", part)
						}
						switch attribute {
						case "PID":
							pid = part
						case "Tasks":
							tasks = part
						}
					}
				}
			}
		}
	}

	if debugLevel > 100 {
		fmt.Println("GetSystemCtlStatus\tpid, tasks:", pid, tasks)
	}
	return pid, tasks
}

// GetSystemCtlStatus runs "systemctl status xtcp.service" and returns pid, tasks, memory (in MBs)
// When called with test=true it reads from a file "./fake_systemctl_status"
func GetSystemCtlStatus(systemctlPath string, testing bool, testfile string) (pid string, tasks string) {

	var lines []string

	// Check the systemctlPath is executable
	if testing != true {
		if misc.CheckFilePermissions(systemctlPath, "0755") == false {
			if debugLevel > 10 {
				log.Fatal("systemctlPath does not have 0755 permissions:", systemctlPath)
			}
		}
	}

	//-----------------------
	// Gather input from system or test file

	// if testing, read from the file, otherwise call out to "systemctl status xtcp.service"
	if testing == true {
		// Otherwise we read a test file with some static output
		lines = misc.ScanFile(testfile) // path is relative to were the "go test" runs
	} else {
		// Create Cmd, buffered output
		// "systemctl status xtcp.service"
		systemctlCmd := cmd.NewCmd(systemctlPath, "status", "xtcp.service")

		// Run and wait for Cmd to return Status
		systemctlOut := <-systemctlCmd.Start()
		if systemctlOut.Error != nil {
			log.Fatalf("GetSystemCtlStatus systemctlCmd.Start error: %s", systemctlOut.Error)
		}
		lines = systemctlOut.Stdout
	}

	pid, tasks = ParseGetSystemCtlStatus(lines)

	return pid, tasks
}

// GetPSStats runs "/bin/ps --no-headers -o pcpu,pmem,rss,sz -p $PID" to grap
// - PCPU %
// - PMEM %
// - RSS ( Resident set size. This is the non-swapped physical memory used by the process)
// - SZ ( Size in RAM pages of the process image)
func GetPSStats(pid string, psPath string, testing bool, testfile string) (pcpu string, pmem string, rss string, sz string) {

	var partToVariablePointer = map[int]*string{
		0: &pcpu,
		1: &pmem,
		2: &rss,
		3: &sz,
	}
	var lines []string

	// Check the psPath is executable
	if testing != true {
		if misc.CheckFilePermissions(psPath, "0755") == false {
			if debugLevel > 10 {
				log.Fatal("psPath does not have 0755 permissions:", psPath)
			}
		}
	}

	//-----------------------
	// Gather input from system or test file

	// if testing, read from the file, otherwise call out to "systemctl status xtcp.service"
	if testing {
		// Otherwise we read a test file with some static output
		lines = misc.ScanFile(testfile) // path is relative to were the "go test" runs
	} else {
		// Create Cmd, buffered output
		// "systemctl status xtcp.service"
		systemctlCmd := cmd.NewCmd(psPath, "--no-headers", "-o", "pcpu,pmem,rss,sz", "-p", pid)

		// Run and wait for Cmd to return Status
		systemctlOut := <-systemctlCmd.Start()
		if systemctlOut.Error != nil {
			log.Fatalf("GetPSStats systemctlCmd.Start error: %s", systemctlOut.Error)
		}
		lines = systemctlOut.Stdout
	}

	// please note that w'er using a loop even though there's only one line :)
	for i, line := range lines {
		if debugLevel > 100 {
			fmt.Println("i:", i, "\tline:", line)
		}
		parts := strings.Fields(line)
		for j, part := range parts {
			if debugLevel > 100 {
				fmt.Println("j:", j, "\tpart:", part)
			}
			*partToVariablePointer[j] = part
		}
	}

	if debugLevel > 100 {
		fmt.Println("GetPSStats\tpcpu, pmem, rss, sz:", pcpu, pmem, rss, sz)
	}
	return pcpu, pmem, rss, sz
}

// XTCPStater sets up Prometheus metrics and emits statsd metrics
// This function polls at *cliFlags.XTCPStaterFrequencySeconds
func XTCPStater(cliFlags cliflags.CliFlags) bool {

	//---------------------------------
	// Register Prometheus metrics

	// Not putting tasks in promethous cos we get this via golang promethous client by default

	promPCPUGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "xtcp",
			Name:      "pcpu",
			Help:      "ps -o pcpu",
		},
	)
	// TODO check if we get this already
	promRSSGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "xtcp",
			Name:      "rss",
			Help:      "ps -o rss",
		},
	)
	// TODO check if we get this already
	promSZGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "xtcp",
			Subsystem: "xtcp",
			Name:      "sz",
			Help:      "ps -o sz",
		},
	)

	// metrics for this go routine
	XTCPStaterUDPs := promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "xtcpStater",
			Name:      "udps",
			Help:      "xtcpStater UDP messages sent (likely to be packets)",
		},
	)
	XTCPStaterUDPBytes := promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "xtcpStater",
			Name:      "udp_bytes",
			Help:      "xtcpStater UDP bytes sent",
		},
	)
	XTCPStaterUDPErrors := promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "xtcp",
			Subsystem: "xtcpStater",
			Name:      "udp_errors",
			Help:      "xtcpStater UDP messages send errors",
		},
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
				fmt.Println("XTCPStater:\tnet.Dial(\"udp\", ", *cliFlags.StatsdDst, ") error:", dialerr)
			}
		}
		defer udpConn.Close()
	}

	//---------------------------------

	var pid, tasks, heapAlloc string
	var pcpu, pmem, rss, sz string
	// https://pkg.go.dev/runtime#MemStats
	mS := new(runtime.MemStats)

	ticker := time.NewTicker(*cliFlags.XTCPStaterFrequency)
	for pollingLoops := 0; ; pollingLoops++ {

		pid, tasks = GetSystemCtlStatus(*cliFlags.XTCPStaterSystemctlPath, false, "") // tests false, so no test file
		pcpu, pmem, rss, sz = GetPSStats(pid, *cliFlags.XTCPStaterPsPath, false, "")

		// https://pkg.go.dev/runtime#ReadMemStats
		runtime.ReadMemStats(mS)
		heapAlloc = strconv.FormatUint(mS.HeapAlloc, 10)

		if *cliFlags.HappyPollerReportModulus == 1 || pollingLoops%*cliFlags.HappyPollerReportModulus == 1 {
			if debugLevel > 10 {
				fmt.Println("XTCPStater\tpid:", pid, "\ttasks:", tasks, "\theapAlloc:", heapAlloc, "\tpcpu:", pcpu, "\tpmem:", pmem, "\trss:", rss, "\tsz:", sz)
			}
		}

		// The get functions return strings, so convert the them to float64 for storing in Prom
		if pcpuF, err := strconv.ParseFloat(pcpu, 64); err == nil {
			promPCPUGauge.Set(pcpuF)
		}
		if rssF, err := strconv.ParseFloat(rss, 64); err == nil {
			promRSSGauge.Set(rssF)
		}
		if szF, err := strconv.ParseFloat(rss, 64); err == nil {
			promSZGauge.Set(szF)
		}

		// Send to statsd
		if !*cliFlags.NoStatsd {
			updateString = fmt.Sprintf("xtcp_heapAlloc:%s|g\nxtcp_pcpu:%s|g\nxtcp_rss:%s|g\nxtcp_sz:%s|g", heapAlloc, pcpu, rss, sz)
			if debugLevel > 100 {
				fmt.Println("XTCPStater:\tupdateString:\n", updateString)
			}
			udpBytesWritten, udpWriteErr = udpConn.Write([]byte(updateString))
			if udpWriteErr != nil {
				XTCPStaterUDPErrors.Inc()
			}
			XTCPStaterUDPs.Inc()
			XTCPStaterUDPBytes.Add(float64(udpBytesWritten))
		}

		<-ticker.C

		// // select not really required, but we could add a HTTP hook or signal handler here
		// select {
		// case _ = <-ticker.C:
		// 	break
		// 	// default:
		// 	// 	//nothing
		// }
	}

}
