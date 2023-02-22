// Package poller contains the main xtcp go routine that fires off the actual kernel polling
//
// This routine is responsible for starting up the netlinkers + inetdiagers
package poller

import (
	"encoding/binary"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/inetdiager"
	"github.com/Edgio/xtcp/pkg/inetdiagerstater"
	"github.com/Edgio/xtcp/pkg/misc"
	"github.com/Edgio/xtcp/pkg/netlinker"
	"github.com/Edgio/xtcp/pkg/netlinkerstater"
	"github.com/Edgio/xtcp/pkg/pollerstater"
	"github.com/Edgio/xtcp/pkg/xtcpnl" // netlink functions

	"golang.org/x/sys/unix"
)

const (
	debugLevel int = 11
)

// cleanWorkerShutdown can be used in the polling loop to shut down the inetdiag workers if required
func cleanWorkerShutdown(inetdiagerWG *sync.WaitGroup, netlinkerCh chan<- netlinker.TimeSpecandInetDiagMessage) {

	// Close the netlinkerCh channel, which will cause the inetdiagers to close
	// because of "for timeSpecandInetDiagMessage := range in {"
	if debugLevel > 10 {
		fmt.Println("cleanWorkerShutdown  close(netlinkerCh)")
	}
	close(netlinkerCh)

	if debugLevel > 10 {
		fmt.Println("cleanWorkerShutdown inetdiagerWG.Wait()")
	}
	inetdiagerWG.Wait()
}

// Poller is instanciated once per address family, and is responsible for:
// 1. Setting up channels and workers
// 2. Sending netlink diag dump requests to the kernel
// 3. Waiting for a done message from the kernel
// 4. Waiting for the netlinkers to complete
// 5. Block waiting for tick
// Left out stats related stuffs
func Poller(af uint8, hostname *string, cliFlags cliflags.CliFlags, wg *sync.WaitGroup, pollerStaterCh chan<- pollerstater.PollerStats, netlinkerStaterCh chan<- netlinkerstater.NetlinkerStatsWrapper, inetdiagerStaterCh chan<- inetdiagerstater.InetdiagerStatsWrapper) {

	defer wg.Done()

	if debugLevel > 10 {
		fmt.Println("poller af:", misc.KernelEnumToString[af], "\tStart")
	}

	// Variables
	//
	// WaitGroups used for shutting down the workers cleanly and safely
	// TODO - To avoid the possibility of multiple polls piling up, for
	// example if our polling frequency is too high, and our processing time is too long
	// then I think we need to add another waitgroup, which will be used if workers are not
	// shutting down.
	// TODO think through this logic.
	var netlinkerWG sync.WaitGroup
	var inetdiagerWG sync.WaitGroup

	// // The report protobuf need to include the time
	// // Opt-ing to use the netlink dump request send time, which is Gettimeofday just before the send
	// // Sadly, we can't easily broadcast the time to the worders, so we're using a mutex
	// // which is set in SendNetlinkDumpRequest, and then read by each netlinker
	// // TODO - consider using something like https://github.com/grafov/bcast
	// var sendNetlinkDumpRequestTimeSpec syscall.Timespec
	// var timeSpecMutex *sync.RWMutex
	// timeSpecMutex = &sync.RWMutex{}

	// Netlink socket variables
	var socketFileDescriptor int
	var socketAddress *unix.SockaddrNetlink
	var netlinkRequest []byte // binary blob containing the netlink inetdiag dump request

	// Channel variables
	// Used to pass from netlinkers to the inetdiagers
	var netlinkerCh chan netlinker.TimeSpecandInetDiagMessage
	// Channel to allow the netlinker which recieves the unix.NLMSG_DONE to signal the poller of this
	var netlinkerRecievedDoneCh chan time.Time

	// Timing variables
	var startPollTime time.Time
	var doneReceivedTime time.Time
	var finishedPollTime time.Time
	var pollToDoneDuration time.Duration
	var pollDuration time.Duration

	var workersStarted bool = false

	// Map addressfamily to number of netlinkers and inetdiagers. TODO iterate
	var afToNetlinkers = map[uint8]*int{
		uint8(2):  cliFlags.Netlinkers4,
		uint8(10): cliFlags.Netlinkers6,
	}
	var afToInetdiagers = map[uint8]*int{
		uint8(2):  cliFlags.Inetdiagers4,
		uint8(10): cliFlags.Inetdiagers6,
	}

	// Prometheus variables
	var currentPollerStats pollerstater.PollerStats

	// Initialize sockets and netlink request binary blobs

	// Build the binary blobs of the netlink inet diag dump requests, one for each address family
	// func BuildNetlinkSockDiagRequest(addressFamily *uint8, make_size int, nlmsg_len int, nlmsg_seq int, nlmsg_pid int)
	netlinkRequest = xtcpnl.BuildNetlinkSockDiagRequest(&af, int(128), uint32(72), uint32(*cliFlags.NlmsgSeq), uint32(0), uint8(0xFF), uint8(0)) // nice works

	// Open the netlink socket using syscall library (rather than golang net package)
	socketFileDescriptor, socketAddress = xtcpnl.OpenNetlinkSocketWithTimeout(*cliFlags.Timeout)
	defer syscall.Close(socketFileDescriptor)

	// Sleeping the IPv6 for 1/2 the pollingLoopFrequencySeconds, so that the polling is offset from IPv4
	// This should mean the overall system impact is spread out more evenly, although obviously because
	// there are so many more IPv4 sockets this isn't really true.
	if af == unix.AF_INET6 {
		if debugLevel > 10 {
			fmt.Println("poller af:", misc.KernelEnumToString[af], "\tSleeping IPv6 poller for:", *cliFlags.PollingFrequency/2)
		}
		time.Sleep(*cliFlags.PollingFrequency / 2)
	}

	// Poller's primary loop
	ticker := time.NewTicker(*cliFlags.PollingFrequency)
	for pollingLoops := 0; misc.MaxLoopsOrForEver(pollingLoops, *cliFlags.MaxLoops); pollingLoops++ {

		if *cliFlags.HappyPollerReportModulus == 1 || pollingLoops%*cliFlags.HappyPollerReportModulus == 1 {
			if debugLevel > 10 {
				fmt.Println("poller af:", misc.KernelEnumToString[af], "\tpollingLoops:", pollingLoops, "\t< Maxloops:", *cliFlags.MaxLoops, "\tworkersStarted:", workersStarted, "\t*netlinkers:", *afToNetlinkers[af], "\t*inetdiagers:", *afToInetdiagers[af])
			}
		}
		currentPollerStats = pollerstater.PollerStats{Af: af, PollingLoops: pollingLoops, PollToDoneDuration: pollToDoneDuration, PollDuration: pollDuration}
		pollerStaterCh <- currentPollerStats

		if workersStarted == false {
			// setup channels
			netlinkerCh = make(chan netlinker.TimeSpecandInetDiagMessage, *cliFlags.NetlinkerChSize)
			netlinkerRecievedDoneCh = make(chan time.Time)

			// startup the workers in reverse pipeline order
			for inetdiagerID := 0; inetdiagerID < *afToInetdiagers[af]; inetdiagerID++ {
				inetdiagerWG.Add(1)
				go inetdiager.Inetdiager(inetdiagerID, &af, netlinkerCh, &inetdiagerWG, *hostname, cliFlags, inetdiagerStaterCh)
				if debugLevel > 100 {
					fmt.Println("poller af:", misc.KernelEnumToString[af], "\tinetdiagerID started:", inetdiagerID)
				}
			}
			workersStarted = true
		}

		// Send NetLink dump request   <-- IMPORTANT!!  This triggers everything else
		if debugLevel > 100 {
			fmt.Println("poller af:", misc.KernelEnumToString[af], "\tsendNetlinkDumpRequest")
		}
		// TODO We are NOT checking return sequence codes
		binary.LittleEndian.PutUint32(netlinkRequest[8:12], uint32(*cliFlags.NlmsgSeq+pollingLoops))
		startPollTime = time.Now()
		xtcpnl.SendNetlinkDumpRequest(socketFileDescriptor, socketAddress, netlinkRequest)

		// Start the netlinkers to consume all the netlink messages
		for netlinkerID := 0; netlinkerID < *afToNetlinkers[af]; netlinkerID++ {
			netlinkerWG.Add(1)
			go netlinker.Netlinker(netlinkerID, &af, socketFileDescriptor, netlinkerCh, netlinkerRecievedDoneCh, &netlinkerWG, startPollTime, cliFlags, netlinkerStaterCh)
		}
		// Blocking here for unix.NLMSG_DONE means there will only ever be a single netlink request/recieve in flight at any time
		// (this also conveniently allows us to grap some timing info)
		doneReceivedTime = <-netlinkerRecievedDoneCh
		pollToDoneDuration = doneReceivedTime.Sub(startPollTime)
		if *cliFlags.HappyPollerReportModulus == 1 || pollingLoops%*cliFlags.HappyPollerReportModulus == 1 {
			if debugLevel > 10 {
				fmt.Println("poller af:", misc.KernelEnumToString[af], "\tpollToDoneDuration:", pollToDoneDuration, "\tpollToDoneDuration.Seconds():", pollToDoneDuration.Seconds())
			}
		}

		// Block waiting for all the netlinkers to finish
		// - The netlinker who gets the DONE will get here first
		// - Then then the other x3 (by default) will get here after timing out on the socket (up to 100ms by default)
		netlinkerWG.Wait()

		// If we're shutting down the inetdiager workers been runs, they shut down here
		// Please note that this will block waiting for the inetdiagerWG sync.WaitGroup to complete
		if *cliFlags.ShutdownWorkers == true {
			cleanWorkerShutdown(&inetdiagerWG, netlinkerCh)
			workersStarted = false
		}

		finishedPollTime = time.Now()
		pollDuration = finishedPollTime.Sub(startPollTime)
		if *cliFlags.HappyPollerReportModulus == 1 || pollingLoops%*cliFlags.HappyPollerReportModulus == 1 {
			if debugLevel > 10 {
				//fmt.Println("poller af:", af, "\tpollDuration:", pollDuration, "\tpollDuration.Seconds():", pollDuration.Seconds())
				fmt.Println("poller af:", misc.KernelEnumToString[af], "\tpollDuration.Seconds():", pollDuration.Seconds(), "\tout of:", *cliFlags.PollingFrequency)
			}
		}

		// Warn if the polling loop is taking more than 80% (constant) of the polling frequency
		if pollDuration > (time.Duration(float64(*cliFlags.PollingFrequency) * *cliFlags.PollingSafetyBuffer)) {
			if debugLevel > 10 {
				fmt.Println("poller af:", misc.KernelEnumToString[af], "\tPOLLING IS TAKING TOO LONG!! WARNING!!")
			}
			// Please note we calculate the pollingLong counter in pollerStats
		}
		// Block until the next tick
		// TODO - block until the next tick or http request
		if debugLevel > 100 {
			fmt.Println("poller af:", misc.KernelEnumToString[af], "\twaiting for ticker at frequency:", *cliFlags.PollingFrequency)
		}
		// TODO We don't really need the select yet, but getting it ready for http hook channel goes here
		//<-ticker.C
		select {
		case _ = <-ticker.C:
			break
			// default:
			// 	//nothing
		}
	}

	// We're all done.  Clean up and get the heck out of here!
	if workersStarted == true {
		cleanWorkerShutdown(&inetdiagerWG, netlinkerCh)
		workersStarted = false
	}

	if debugLevel > 10 {
		fmt.Println("poller af:", af, "\tDone")
	}

}
