// Package netlinker is the netlinker go routine of the xtcp package
//
// Netlinker recieves netlink messages from the kernel and passes
// the discrete messages to the inetdiagers workers
package netlinker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/inetdiag"
	"github.com/Edgio/xtcp/pkg/netlinkerstater"
	"golang.org/x/sys/unix"
)

const (
	debugLevel int = 11
)

// TimeSpecandInetDiagMessage struct is the message that is sent from the recvfrom to the NetLink Message workers
// This includes the timeSpec which is the time the netlink dump request was sent (or really just before that)
type TimeSpecandInetDiagMessage struct {
	TimeSpec        syscall.Timespec //https://golang.org/pkg/syscall/#Timespec
	InetDiagMessage []byte
}

// TODO move to slice of slice
//InetDiagMessage [][]byte

//https://golang.org/pkg/syscall/#Timespec
// type Timespec struct {
//     Sec  int64
//     Nsec int64
// }

// CheckNetlinkMessageType checks for netlink message types NLMSG_NOOP, NLMSG_DONE, NLMSG_ERROR, NLMSG_OVERRUN
func CheckNetlinkMessageType(id int, af *uint8, Type uint16) (netlinkMsgComplete bool, netlinkMsgDone bool, errorCount int) {

	switch Type {
	case unix.NLMSG_NOOP:
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tunix.NLMSG_NOOP")
		}
		errorCount++
		netlinkMsgComplete = true
		break
	case unix.NLMSG_DONE:
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tunix.NLMSG_DONE") // Yay!
		}
		netlinkMsgDone = true
		netlinkMsgComplete = true
		break
	case unix.NLMSG_ERROR:
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tunix.NLMSG_ERROR")
		}
		errorCount++
		netlinkMsgComplete = true
		break
	case unix.NLMSG_OVERRUN:
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tunix.NLMSG_OVERRUN")
		}
		errorCount++
		netlinkMsgComplete = true
		break
	default:
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Type default")
		}
	}
	return netlinkMsgComplete, netlinkMsgDone, errorCount
}

// Netlinker makes the syscall to read from the netlink socket
// Then we break the netlink messages up into their Inetdiag messages, and stream to the downstream workers
// over the channel.
//
// TODO - Currently we're doing a single x1 inetdiag message on the channel at a time.
// profile.TraceProfile does show gaps between messages, so we should test batching.
// (Probably should add a slice capability with a configurable max length)
//
// For the purposes to checking to see if it's better to close the pipeline workers down between
// netlink inet_diag dump requests, or not, I've added logic here to allow this worker to close or not
// We're basically trying to understand if respawning goroutines for this work is better/worse
// and holding them open the whole time.  Obviously this is really going to depend on lots of factors,
// like polling frequency, etc.
//
// The thing about the syscall.Recvfrom is that we can't do a "select" on this with a channel
// at the same time.  Therefore, to avoid any race conditions, the syscall.Recvfrom has timeout,
// so the operating system will return every <netlinkSocketTimeout> seconds, which will allow
// us to close down this worker if configured to do so.
//
// Specificlally, to allow the netlinker to be shutdown, or held open, I recently
// added the "for socketTimeoutCount" loop, so if shutdownWorkers is false, we'll just
// keep re-issuing the syscall.Recvfrom and blocking until the timeout.
//
// Recommend not setting the timeout too low,
// or your just going to thrash with system calls.  Similarly, probably don't run too many netlinker,
// workers.
// With x4 workers and 5 second timeout seems reasonable.
func Netlinker(id int, af *uint8, socketFileDescriptor int, out chan<- TimeSpecandInetDiagMessage, netlinkerRecievedDoneCh chan<- time.Time, wg *sync.WaitGroup, startTime time.Time, cliFlags cliflags.CliFlags, netlinkerStaterCh chan<- netlinkerstater.NetlinkerStatsWrapper) {

	defer wg.Done()

	var packetBuffer []byte
	var packetsProcessed int
	var netlinkMsgHeader inetdiag.NlMsgHdr
	var packetBufferBytesRemaining int
	var packetBufferBytesRead int
	var packetBufferInSizeTotal int
	var netlinkMsgCountTotal int
	var packetBufferBytesReadTotal int
	var inetdiagMsgCopyBytesTotal int
	var nastyContinue int
	var netlinkMsgErrorCount int
	var outBlocked int
	var blockedStartTime time.Time
	var blockedDuration time.Duration
	var longestBlockedDuration time.Duration

	//** is not double pointer.  it is multiply by pointer.
	if *cliFlags.PacketSize == 0 {
		packetBuffer = make([]byte, syscall.Getpagesize()**cliFlags.PacketSizeMply)
	} else {
		packetBuffer = make([]byte, *cliFlags.PacketSize**cliFlags.PacketSizeMply)
	}

	if debugLevel > 100 {
		fmt.Println("netlinker:", id, "\taf:", *af, "\tbinary.size(packetBuffer):", binary.Size(packetBuffer))
	}

	var packetsProcessingnetlinkerDone = false
	for packetsProcessed = 0; !packetsProcessingnetlinkerDone; packetsProcessed++ {
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketsProcessed:", packetsProcessed, "\tsyscall.Recvfrom called")
		}
		packetBufferInSize, _, err := syscall.Recvfrom(socketFileDescriptor, packetBuffer, 0)

		if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
			if debugLevel > 100 {
				fmt.Println("netlinker:", id, "\taf:", *af, "\tsyscall.Recvfrom timeout")
			}
			packetsProcessingnetlinkerDone = true
			break //This is where we can break out from a timeout if the socket has timeout configured
		}
		if err != nil {
			if debugLevel > 100 {
				fmt.Println("netlinker:", id, "\taf:", *af, "\tunix.Recvfrom:", err)
				//log.Fatalf("unix.Recvfrom", err)
			}
			nastyContinue++
			continue // continuing is a bit nasty, but is probably safe enough
		}

		// Little sanity check
		if packetBufferInSize < unix.NLMSG_HDRLEN {
			if debugLevel > 100 {
				//log.Fatalf("netlinker:", id, "\tNLMSG_HDRLEN.Recvfrom too small") // fatal
				fmt.Println("netlinker:", id, "\taf:", *af, "\tNLMSG_HDRLEN.Recvfrom too small")
			}
			nastyContinue++
			continue // continuing is a bit nasty, but is probably safe enough
		}

		packetBufferInSizeTotal += packetBufferInSize
		packetBufferBytesRemaining = packetBufferInSize
		packetBufferBytesRead = 0
		packetReader := bytes.NewReader(packetBuffer)
		if debugLevel > 100 {
			fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferInSize:", packetBufferInSize, "\tpacketsProcessed:", packetsProcessed, "\tpacketBufferInSizeTotal(M):", packetBufferInSizeTotal/10^6, "\tnetlinkMsgCountTotal:", netlinkMsgCountTotal)
		}
		var netlinkMsgComplete = false
		for netlinkMsgCount := 0; !netlinkMsgComplete && packetBufferBytesRemaining >= syscall.NLMSG_HDRLEN; netlinkMsgCount++ {

			err := binary.Read(packetReader, binary.LittleEndian, &netlinkMsgHeader)
			if err != nil {
				if debugLevel > 100 {
					fmt.Println("binary.Read netlinkMsgHeader failed:", err)
				}
				netlinkMsgComplete = true
				if err == io.EOF {
					if debugLevel > 100 {
						fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketReader EOF")
					}
				}
				break
			}
			packetBufferBytesRead += binary.Size(netlinkMsgHeader)
			packetBufferBytesRemaining -= binary.Size(netlinkMsgHeader)
			if debugLevel > 100 {
				fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRead:", packetBufferBytesRead)
				fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRemaining:", packetBufferBytesRemaining)

				fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Length:", netlinkMsgHeader.Length)
				fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Type:", netlinkMsgHeader.Type)
				fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Flags:", netlinkMsgHeader.Flags)
			}

			var errorCount int
			var netlinkMsgDone bool
			netlinkMsgComplete, netlinkMsgDone, errorCount = CheckNetlinkMessageType(id, af, netlinkMsgHeader.Type)
			if errorCount > 0 {
				netlinkMsgErrorCount += errorCount
			}
			if netlinkMsgDone {
				netlinkerRecievedDoneCh <- time.Now() // DONE!!
			}
			if netlinkMsgComplete {
				break
			}

			switch netlinkMsgHeader.Flags {
			case unix.NLM_F_MULTI:
				if debugLevel > 100 {
					fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Flags unix.NLM_F_MULTI")
				}

				// Make a copy of the data of the packet and send to the next layer in the pipeline
				var timeSpecandInetDiagMessageCopy TimeSpecandInetDiagMessage
				// We're using timeSpec 64bit to match the kernel
				// https://github.com/torvalds/linux/blob/458ef2a25e0cbdc216012aa2b9cf549d64133b08/include/linux/time64.h#L13

				// Please UnixNano() includes the .Unix() seconds
				// https://golang.org/pkg/time/#Time.UnixNano which includes the seconds
				var tempTime int64
				tempTime = startTime.UnixNano()
				timeSpecandInetDiagMessageCopy.TimeSpec.Sec, timeSpecandInetDiagMessageCopy.TimeSpec.Nsec = tempTime/1e9, tempTime%1e9 //note seconds, and nanos split out here
				if debugLevel > 1000 {
					fmt.Println("netlinker:", id, "\taf:", *af, "\ttimeSpecandInetDiagMessageCopy.TimeSpec.Sec:", timeSpecandInetDiagMessageCopy.TimeSpec.Sec)
					fmt.Println("netlinker:", id, "\taf:", *af, "\ttimeSpecandInetDiagMessageCopy.TimeSpec.Nsec:", timeSpecandInetDiagMessageCopy.TimeSpec.Nsec)
				}
				timeSpecandInetDiagMessageCopy.InetDiagMessage = make([]byte, int(netlinkMsgHeader.Length)-binary.Size(netlinkMsgHeader))

				err := binary.Read(packetReader, binary.LittleEndian, &timeSpecandInetDiagMessageCopy.InetDiagMessage)
				if err != nil {
					if debugLevel > 100 {
						fmt.Println("netlinker:", id, "\taf:", *af, "\tbinary.Read inetdiagMsgCopy failed:", err)
					}
					if err == io.EOF {
						if debugLevel > 100 {
							fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketReader EOF")
						}
						break
					}
				}
				packetBufferBytesRead += binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage)
				packetBufferBytesRemaining -= binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage)
				packetBufferBytesReadTotal += binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage)

				if *cliFlags.SamplingModulus == 1 || netlinkMsgCount%*cliFlags.SamplingModulus == 1 {
					// This was originally just "out <- inetdiagMsgCopy", but using select per https://blog.golang.org/pipelines
					// It's better golang practise to do this via select.  whichever is non-blocking first will proceed.
					select {
					// send to the next level
					//case out <- inetdiagMsgCopy:
					case out <- timeSpecandInetDiagMessageCopy:
						if debugLevel > 100 {
							fmt.Println("netlinker:", id, "\taf:", *af, "\tsent inetdiagMsgCopy\tbinary.Size(inetdiagMsgCopy):", binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage), "\tnetlinkMsgCount:", netlinkMsgCount, "\tpacketsProcessed:", packetsProcessed)
						}
						if debugLevel > 100 {
							fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRead:", packetBufferBytesRead)
							fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRemaining:", packetBufferBytesRemaining)
						}
						inetdiagMsgCopyBytesTotal += binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage)
						break
						// We could include the netlinkerDone here, which would allow the worker to shutdown before finishing processing the full netlink message
						// Not going to do that currently though, as this is cleaner.
						// case _ = <-netlinkerDone:
						// 	recvfromWorkernetlinkerDone = true
						// 	break
					default:
						// Default will catch the case where the above send on the channel will block.
						// This is important to track because it means we'll know if the channel size (promPollerChSize) is too small
						blockedStartTime = time.Now()
						outBlocked++
						out <- timeSpecandInetDiagMessageCopy //block
						blockedDuration = time.Since(blockedStartTime)
						if blockedDuration > longestBlockedDuration {
							longestBlockedDuration = blockedDuration
						}
						if debugLevel > 100 {
							fmt.Println("netlinker:", id, "\taf:", *af, "\tsent inetdiagMsgCopy\tbinary.Size(inetdiagMsgCopy):", binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage), "\tnetlinkMsgCount:", netlinkMsgCount, "\tpacketsProcessed:", packetsProcessed)
						}
						if debugLevel > 100 {
							fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRead:", packetBufferBytesRead)
							fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRemaining:", packetBufferBytesRemaining)
						}
						inetdiagMsgCopyBytesTotal += binary.Size(timeSpecandInetDiagMessageCopy.InetDiagMessage)
					}
				}

				if packetBufferBytesRemaining == 0 {
					if debugLevel > 100 {
						fmt.Println("netlinker:", id, "\taf:", *af, "\tpacketBufferBytesRemaining ZERO! Next packet please!")
					}
					// we don't really need to set this because of the packetBufferBytesRemaining size test, but hopefully it's more clear this way
					netlinkMsgComplete = true
				}
			default:
				if debugLevel > 100 {
					fmt.Println("netlinker:", id, "\taf:", *af, "\tnetlinkMsgHeader.Flags default")
				}
				netlinkMsgErrorCount++ //going to increment this error counter, so we can see if this ever happens
				break
			}
			//switch netlinkMsgHeader.Flags {
			netlinkMsgCountTotal++
		}
		//for netlinkMsgCount := 0 ; !netlinkMsgComplete && packetBufferBytesRemaining >= syscall.NLMSG_HDRLEN; netlinkMsgCount++ {
	}
	//for packetsProcessed := 0; !packetsProcessingnetlinkerDone; packetsProcessed++ {

	netlinkerStaterCh <- netlinkerstater.NetlinkerStatsWrapper{
		Af: *af,
		ID: id,
		Stats: netlinkerstater.NetlinkerStats{
			PacketsProcessed:           packetsProcessed,
			NastyContinue:              nastyContinue,
			PacketBufferInSizeTotal:    packetBufferInSizeTotal,
			NetlinkMsgCountTotal:       netlinkMsgCountTotal,
			PacketBufferBytesReadTotal: packetBufferBytesReadTotal,
			InetdiagMsgCopyBytesTotal:  inetdiagMsgCopyBytesTotal,
			NetlinkMsgErrorCount:       netlinkMsgErrorCount,
			OutBlocked:                 outBlocked,
			LongestBlockedDuration:     longestBlockedDuration,
		},
	}

	if debugLevel > 100 {
		fmt.Println("netlinker:", id, "\taf:", *af, "\tclose")
	}
}
