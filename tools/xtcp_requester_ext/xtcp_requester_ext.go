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

	var packetBuffer []byte
	packetBuffer = make([]byte, syscall.Getpagesize()*8)

	for i := 0; i < 256; i++ {
		netlinkRequest = xtcpnl.BuildNetlinkSockDiagRequest(&addressFamily, int(128), uint32(72), uint32(i), uint32(0), uint8(i), uint8(0))
		xtcpnl.SendNetlinkDumpRequest(socketFileDescriptor, socketAddress, netlinkRequest)
		fmt.Println("requester i:", i, "\tsent")

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

		time.Sleep(100 * time.Millisecond)
	}
}
