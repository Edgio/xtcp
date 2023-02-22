//
// This package contains netlink related functions for opening netlinks sockets,
// building netlink messages, and sending netlink messages
//
// openNetlinkSocketWithTimeout - opens netlink socket using syscalls
// buildNetlinkSockDiagRequest - builds binary blobs to send to the netlink socket (unsafe)
// sendNetlinkDumpRequest - sends a netlink inetdiag dump request
//
// These functions will log.Fatalf if they fail
// pretty horrible has happened if you can't get a netlink socket or send to it.

package xtcpnl

//import "github.com/Edgio/xtcp/xtcpnl" // netlink related functions

import (
	"encoding/binary"
	"fmt"
	"log"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	debugLevel int = 11 //101 //11
)

// OpenNetlinkSocketWithTimeout function opens a Netlink socket in the C style way
// Using the newer https://godoc.org/golang.org/x/sys/unix library
// Commented out is some timeout code, which was used during testing,
// but leaving it here in case we want it back at some point
func OpenNetlinkSocketWithTimeout(timeout int64) (socketFileDescriptor int, socketAddress *unix.SockaddrNetlink) {

	if debugLevel > 100 {
		fmt.Println("OpenNetlinkSocketWithTimeout\ttimeout:", timeout)
	}
	// Create netlink socket
	// This is using the newer library: https://godoc.org/golang.org/x/sys/unix#Socket
	socketFileDescriptor, err := syscall.Socket(
		unix.AF_NETLINK,
		unix.SOCK_DGRAM,
		unix.NETLINK_INET_DIAG,
	)
	if err != nil {
		log.Fatalf("OpenNetlinkSocketWithTimeout unix.Socket %s", err)
	}

	// Bind the socket
	// https://godoc.org/golang.org/x/sys/unix#SockaddrNetlink
	socketAddress = &unix.SockaddrNetlink{Family: syscall.AF_NETLINK}
	// https://godoc.org/golang.org/x/sys/unix#Bind
	err = unix.Bind(socketFileDescriptor, socketAddress)
	if err != nil {
		log.Fatalf("OpenNetlinkSocketWithTimeout unix.Bind %s", err)
	}

	// Set socket timeout based on constants
	// doing this so that netlinkers can close on their own (or in the very unlikely event the kernel doesn't respond)
	if timeout != 0 {
		// https://godoc.org/golang.org/x/sys/unix#SetsockoptTimeval
		var tv syscall.Timeval
		if timeout >= 1000 {
			// seconds
			tv.Sec = timeout / 1000
		} else {
			// milliseconds
			tv.Usec = timeout * 1000 // microsecond or 1 millionth of a second.  1 milliseconds = 1000 micro
		}
		if debugLevel > 100 {
			fmt.Println("OpenNetlinkSocketWithTimeout\ttv:", tv)
		}
		err = syscall.SetsockoptTimeval(socketFileDescriptor, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
		if err != nil {
			log.Fatalf("OpenNetlinkSocketWithTimeout SetsockopttimeSpec %s", err)
		}
	}

	return socketFileDescriptor, socketAddress
}

// BuildNetlinkSockDiagRequest function builds up the binary bytes for the Netlink request
// We're using unsafe pointers for the uint8, because there is no PutUint8
// addressFamily should be 2=IPv4, and 10=IPv6 per the kernel
// TODO - switch to binary package, because we're using unsafe.  This is the only unsafe code in this program.
// Lots of comments here to show what we're doing, and includes links to the kernel source
func BuildNetlinkSockDiagRequest(addressFamily *uint8, make_size int, nlmsg_len uint32, nlmsg_seq uint32, nlmsg_pid uint32, idiag_ext uint8, idiag_stats uint8) (packetBytes []byte) {
	// Statically build up the netlink socket diag request
	// TODO - use binary.size in stead of constants here
	//packetBytes = make([]byte, 72+56) //128
	packetBytes = make([]byte, make_size)

	// https://github.com/torvalds/linux/blob/1d51b4b1d3f2db0d6d144175e31a84e472fbd99a/tools/include/uapi/linux/netlink.h#L44
	// struct nlmsghdr {
	// 	__u32		nlmsg_len;	/* Length of message including header */
	// 	__u16		nlmsg_type;	/* Message content */
	// 	__u16		nlmsg_flags;	/* Additional flags */
	// 	__u32		nlmsg_seq;	/* Sequence number */
	// 	__u32		nlmsg_pid;	/* Sending process port ID */
	// };
	//binary.LittleEndian.PutUint32(packetBytes[0:4], uint32(0)) // constant hack for the length
	binary.LittleEndian.PutUint32(packetBytes[0:4], uint32(nlmsg_len)) // constant hack for the length
	//binary.LittleEndian.PutUint32(packetBytes[0:4], uint32(128)) // constant hack for the length
	//binary.LittleEndian.PutUint32(packetBytes[0:4], uint32(168)) // constant hack for the length
	binary.LittleEndian.PutUint16(packetBytes[4:6], uint16(20)) // #define SOCK_DIAG_BY_FAMILY 20  in uapi/linux/sock_diag.h
	binary.LittleEndian.PutUint16(packetBytes[6:8], uint16(syscall.NLM_F_DUMP|syscall.NLM_F_REQUEST))
	binary.LittleEndian.PutUint32(packetBytes[8:12], uint32(nlmsg_seq))
	binary.LittleEndian.PutUint32(packetBytes[12:16], uint32(nlmsg_pid)) // not using pid

	//https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L38
	// struct inet_diag_req_v2 {
	// 	__u8	sdiag_family;
	// 	__u8	sdiag_protocol;
	// 	__u8	idiag_ext;
	// 	__u8	pad;
	// 	__u32	idiag_states;
	// 	struct inet_diag_sockid id;
	// };
	//https://github.com/torvalds/linux/blob/481ed297d900af0ce395f6ca8975903b76a5a59e/include/linux/socket.h#L165
	//#define AF_INET		2	/* Internet IP Protocol 	*/
	//#define AF_INET6	10	/* IP version 6			*/

	// There is no PutUint8
	//binary.LittleEndian.PutUint8(packetBytes[16:17], uint8(*addressFamily))   // #define AF_INET		2
	//binary.LittleEndian.PutUint8(packetBytes[17:18], uint8(unix.IPPROTO_TCP)) // IPPROTO_TCP = 6
	*(*uint8)(unsafe.Pointer(&packetBytes[16:17][0])) = uint8(*addressFamily)   // #define AF_INET      2
	*(*uint8)(unsafe.Pointer(&packetBytes[17:18][0])) = uint8(unix.IPPROTO_TCP) // IPPROTO_TCP = 6
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_MEMINFO-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_INFO-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_VEGASINFO-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_CONG-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_TOS-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_TCLASS-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_SKMEMINFO-1));
	// inet_diag_req_v2.idiag_ext |= (1<<(INET_DIAG_SHUTDOWN-1));

	// 8 7 6 5 4 3 2 1 0
	//                 0 INET_DIAG_NONE,
	//               1   INET_DIAG_MEMINFO,
	//             2     INET_DIAG_INFO,       <-- want
	//           3       INET_DIAG_VEGASINFO,  <-- want
	//         4         INET_DIAG_CONG,       <-- want
	//       5           INET_DIAG_TOS,
	//     6             INET_DIAG_TCLASS,
	//   7               INET_DIAG_SKMEMINFO,  <-- want
	// 8                 INET_DIAG_SHUTDOWN,
	// 7 4 3 2
	// = 2^7 + 2^4 + 2^3 + 2^2 = 156

	// There is no PutUint8
	//binary.LittleEndian.PutUint8(packetBytes[18:19], uint8(0xFF)) // hack just light up the bits, instead of the complex bit shifts above   <--- Request everything!!
	//binary.LittleEndian.PutUint8(packetBytes[19:20], uint8(0))    //pad
	*(*uint8)(unsafe.Pointer(&packetBytes[18:19][0])) = uint8(idiag_ext)   // hack just light up the bits, instead of the complex bit shifts above   <--- Request everything!!
	*(*uint8)(unsafe.Pointer(&packetBytes[19:20][0])) = uint8(idiag_stats) // pad

	// Which TCP socket states?
	// https://github.com/torvalds/linux/blob/2f4c53349961c8ca480193e47da4d44fdb8335a8/include/net/tcp_states.h
	binary.LittleEndian.PutUint32(packetBytes[20:24], uint32(1<<1)) // established

	// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L14
	// 	/* Socket identity */
	// struct inet_diag_sockid {
	// 	__be16	idiag_sport;
	// 	__be16	idiag_dport;
	// 	__be32	idiag_src[4];
	// 	__be32	idiag_dst[4];
	// 	__u32	idiag_if;
	// 	__u32	idiag_cookie[2];
	// #define INET_DIAG_NOCOOKIE (~0U)
	// };
	// binary.LittleEndian.PutUint16(packetBytes[24:26], uint16(0)) //sport
	// binary.LittleEndian.PutUint16(packetBytes[26:28], uint16(0)) //dport
	// binary.LittleEndian.PutUint32(packetBytes[28:32], uint32(0)) //src
	// binary.LittleEndian.PutUint32(packetBytes[32:36], uint32(0)) //dst
	// binary.LittleEndian.PutUint32(packetBytes[36:40], uint32(0)) //if
	// binary.LittleEndian.PutUint32(packetBytes[40:44], uint32(0)) //cookie

	return packetBytes
}

// SendNetlinkDumpRequest function sends the netlink request
// Please note the mutex is for being able to update the netlink revc function with the time we sent the request
// This is described in more detail in the xtcp.go
// TODO - look to refactor the arguments here using a type/struct which would probably be easier to read
func SendNetlinkDumpRequest(socketFileDescriptor int, socketAddress *unix.SockaddrNetlink, packetBytes []byte) {
	// Send the netlink dump request
	// https://godoc.org/golang.org/x/sys/unix#Sendto
	err := unix.Sendto(socketFileDescriptor, packetBytes, 0, socketAddress)
	if err != nil {
		log.Fatalf("unix.Sendto:%s", err)
	}
}
