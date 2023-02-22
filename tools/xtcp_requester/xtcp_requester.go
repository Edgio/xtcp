package main

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/Edgio/xtcp/pkg/xtcpnl" // netlink related functions
	"golang.org/x/sys/unix"
)

func main() {

	// Netlink socket variables
	var socketFileDescriptor int
	var socketAddress *unix.SockaddrNetlink
	var timeout int64 = 100
	var addressFamily uint8 = unix.AF_INET

	// Open the netlink socket using syscall library (rather than golang net package)
	socketFileDescriptor, socketAddress = xtcpnl.OpenNetlinkSocketWithTimeout(timeout)
	defer syscall.Close(socketFileDescriptor)

	var netlinkRequest []byte

	make_sizes := [...]int{128, 168}
	nlmsg_lens := [...]int{72, 128, 168}
	nlmsg_seqs := [...]int{0, 666, 123456}
	nlmsg_pids := [...]int{0, 666}

	var packetBuffer []byte
	packetBuffer = make([]byte, syscall.Getpagesize()*8)

	var testNumber int
	for i := 0; i < 1; i++ {
		for _, make_size := range make_sizes {
			for _, nlmsg_len := range nlmsg_lens {
				for _, nlmsg_seq := range nlmsg_seqs {
					for _, nlmsg_pid := range nlmsg_pids {
						netlinkRequest = xtcpnl.BuildNetlinkSockDiagRequest(&addressFamily, make_size, nlmsg_len, nlmsg_seq, nlmsg_pid, 0xFF, 0)
						xtcpnl.SendNetlinkDumpRequest(socketFileDescriptor, socketAddress, netlinkRequest)
						fmt.Println("requester i:", i, "\ttestNumber:", testNumber, "\taddressFamily:", addressFamily, "\tmake_size:", make_size, "\tnlmsg_len:", nlmsg_len, "\tnlmsg_seq:", nlmsg_seq, "\tnlmsg_pid:", nlmsg_pid)
						testNumber++

						for x := 0; x < 100000; x++ {
							packetBufferInSize, _, err := syscall.Recvfrom(socketFileDescriptor, packetBuffer, 0)
							if 1 == 2 {
								fmt.Println("packetBufferInSize", packetBufferInSize)
							}

							if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
								fmt.Println("syscall.Recvfrom timeout\tx:", x)
								break //This is where we can break out from a timeout if the socket has timeout configured
							}
							if err != nil {
								fmt.Println("unix.Recvfrom:", err)
								continue
							}
						}

						time.Sleep(1 * time.Second)
					}
				}
			}
		}
	}

}
