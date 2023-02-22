// Package inetdiager is the inetdiager go routine of the xtcp package
//
// This does the heavy lifting of processing the netlink messages
package inetdiager

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
	"github.com/Edgio/xtcp/pkg/inetdiagerstater"
	"github.com/Edgio/xtcp/pkg/netlinker"
	"github.com/Edgio/xtcp/pkg/xtcppb"
	"github.com/nsqio/go-nsq"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	debugLevel int = 11
)

// swapUint16 converts a uint16 to network byte order and back.
// Stolen from: https://github.com/tsuna/endian/blob/master/little.go
// This is used to avoid multiple binary reads for the tcp info, because the TCP port numbers
// are the only bigendian (network byte order) variables jammed in the middle of the struct
// This is used in inetdiager
// TODO - tests
// TODO - make this entire code endian correct on all platforms
// TODO Could switch this out to binary.Read binary.BigEndian
func SwapUint16(n uint16) uint16 {
	return (n&0x00FF)<<8 | (n&0xFF00)>>8
}

// notDecodingThisAttributeTypeYet function is called for attribute types we do not yet handle
// Just a placeholder which should be switched to binaryReadWithErrorHandling()
// when the parsing strategy is clear
func notDecodingThisAttributeTypeYet() (inetdiagMsgComplete bool, bytesRead int) {
	//inetdiagMsgComplete, attributesBytesRead = false, 0
	return false, 0
}

// Function is called by the inetdiagers to do the binary.Read() of a variable and error handling
// This function simplifies the giant nlattr.NlaType block significantly
// (This function probably gets inlined by the compiler)
func binaryReadWithErrorHandling(id int, inetDiagString string, myReader io.Reader, data interface{}, netlinkAttributeDataLength int, af *uint8) (inetdiagMsgComplete bool, bytesRead int) {
	inetdiagMsgComplete = false

	// Check we aren't going to try to read too much (this is mostly for NET_DIAG CONG, but is a safety check)
	if binary.Size(data) > netlinkAttributeDataLength {
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\t", inetDiagString, " binary.Size(data):", binary.Size(data), " > netlinkAttributeDataLength:", netlinkAttributeDataLength)
		}
		// Hack TODO review
		//inetdiagMsgComplete = true
		//return inetdiagMsgComplete, 0
	} else {
		if debugLevel > 100 {
			if inetDiagString == "INET_DIAG_INFO" {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\t", inetDiagString, " binary.Size(data):", binary.Size(data), " <= netlinkAttributeDataLength:", netlinkAttributeDataLength)
			}
		}
	}

	// This ultimately ends up with io.ReadFull, with the binary.Read helping with Endianness (which I'm probably breaking)
	// To do - add endianess tests and then do the Endianness dynamically here
	err := binary.Read(myReader, binary.LittleEndian, data)
	if err != nil {
		if debugLevel > 10 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\t", inetDiagString, " binary.Read failed:", err)
		}
		if err == io.EOF {
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\t", inetDiagString)
			}
		}
		inetdiagMsgComplete = true
	}
	bytesRead = binary.Size(data)
	if debugLevel > 100 {
		fmt.Println("inetdiager:", id, "\taf:", *af, "\t", inetDiagString, "\tinetdiagMsgComplete:", inetdiagMsgComplete, "\tbytesRead:", bytesRead)
	}
	return inetdiagMsgComplete, bytesRead
}

// Little debug println helper function
func printInetDiagMsg(id int, af *uint8, inetdiagMsg inetdiag.InetDiagMsg, sourceIP net.IP, destinationIP net.IP) {
	if debugLevel > 100 {
		fmt.Println("inetdiager:", id, "\taf:", *af, "\tSource:", sourceIP, ":", inetdiagMsg.SocketID.SourcePort, "\tDestination:", destinationIP, ":", inetdiagMsg.SocketID.DestinationPort)
	}
}

// This function does the copying and data type conversion from the kernel type to the protobuf types
// This is because the protos smallest integer type is the uint32, and in many cases the kernel is using something smaller
func buildProto(id int, af *uint8, timeSpec *syscall.Timespec, hostname *string, inetdiagMsg *inetdiag.InetDiagMsg, sourceIPbytes []byte, destinationIPbytes []byte, meminfo *inetdiag.MemInfo, tcpinfo *inetdiag.TCPInfo415, congestionAlgorithm *string, shutdownState *uint8, typeOfService *uint8, trafficClass *uint8, skmeminfo *inetdiag.SkMemInfo, bbrinfo *inetdiag.BBRInfo, classID *uint32, sndWscale *uint32, rcvWscale *uint32, report bool, deliveryRateAppLimited *uint32, fastOpenClientFail *uint32) *xtcppb.XtcpRecord {

	// convert kernel uint8s to uint32s (which is the minimum size for proto buf data types)
	var familyu32 = uint32(inetdiagMsg.Family)
	var stateu32 = uint32(inetdiagMsg.State)
	var timeru32 = uint32(inetdiagMsg.Timer)
	var retransu32 = uint32(inetdiagMsg.Retrans)

	var sourceportu32 = uint32(inetdiagMsg.SocketID.SourcePort)
	var destinationportu32 = uint32(inetdiagMsg.SocketID.DestinationPort)

	var tcpinfoStateu32 = uint32(tcpinfo.State)
	var castateu32 = uint32(tcpinfo.CaState)
	var retransmitsu32 = uint32(tcpinfo.Retransmits)
	var probesu32 = uint32(tcpinfo.Probes)
	var backoffu32 = uint32(tcpinfo.Backoff)
	var optionsu32 = uint32(tcpinfo.Options)

	// The protobuf stores congestion algorithm as enum
	// enum CongestionAlgorithm {
	//     UNKNOWN = 0;
	//     CUBIC = 1;
	//     BBR1 = 2;
	//     BBR2 = 3;
	// }
	// https://blog.golang.org/maps
	// The full "cubic" didn't match for some reason.  Possibly because there's a trailing character?
	// Anyway, matching using the first three (3) chars works fine. TODO
	m := map[string]xtcppb.XtcpRecordCongestionAlgorithm{
		"cub": xtcppb.XtcpRecord_CUBIC,
		"bbr": xtcppb.XtcpRecord_BBR1,
	}
	congestionAlgorithmEnum := m[*congestionAlgorithm]
	if debugLevel > 100 {
		fmt.Println("inetdiager:", id, "\taf:", *af, "\tcongestionAlgorithm:", *congestionAlgorithm, "x\tcongestionAlgorithmEnum:", congestionAlgorithmEnum)
	}

	if debugLevel > 1000 {
		fmt.Println("inetdiager:", id, "\taf:", *af, "familyu32:", familyu32)
	}

	XtcpRecord := &xtcppb.XtcpRecord{
		Hostname: hostname,
		EpochTime: &xtcppb.Timespec64T{
			Sec:  &timeSpec.Sec,
			Nsec: &timeSpec.Nsec,
		},
		InetDiagMsg: &xtcppb.InetDiagMsg{
			Family:  &familyu32,
			State:   &stateu32,
			Timer:   &timeru32,
			Retrans: &retransu32,
			SocketID: &xtcppb.SocketID{
				SourcePort:      &sourceportu32,
				DestinationPort: &destinationportu32,
				Source:          sourceIPbytes,
				Destination:     destinationIPbytes,
				Interface:       &inetdiagMsg.SocketID.Interface,
				Cookie:          &inetdiagMsg.SocketID.Cookie,
			},
			Expires: &inetdiagMsg.Expires,
			Rqueue:  &inetdiagMsg.Rqueue,
			Wqueue:  &inetdiagMsg.Wqueue,
			UID:     &inetdiagMsg.UID,
			Inode:   &inetdiagMsg.Inode,
		},
		// MemInfo: &xtcppb.MemInfo{
		// 	Rmem: &meminfo.Rmem,
		// 	Wmem: &meminfo.Wmem,
		// 	Fmem: &meminfo.Fmem,
		// 	Tmem: &meminfo.Tmem,
		// },
		TcpInfo: &xtcppb.TcpInfo{
			State:                  &tcpinfoStateu32,
			CaState:                &castateu32,
			Retransmits:            &retransmitsu32,
			Probes:                 &probesu32,
			Backoff:                &backoffu32,
			Options:                &optionsu32,
			SendScale:              sndWscale,
			RcvScale:               rcvWscale,
			DeliveryRateAppLimited: deliveryRateAppLimited,
			// TODO fix for kernel 5+
			//	FastOpenClientFailed:   fastOpenClientFail,
			Rto:           &tcpinfo.Rto,
			Ato:           &tcpinfo.Ato,
			SndMss:        &tcpinfo.SndMss,
			RcvMss:        &tcpinfo.RcvMss,
			Unacked:       &tcpinfo.Unacked,
			Sacked:        &tcpinfo.Sacked,
			Lost:          &tcpinfo.Lost,
			Retrans:       &tcpinfo.Retrans,
			Fackets:       &tcpinfo.Fackets,
			LastDataSent:  &tcpinfo.LastDataSent,
			LastAckSent:   &tcpinfo.LastAckSent,
			LastDataRecv:  &tcpinfo.LastDataRecv,
			LastAckRecv:   &tcpinfo.LastAckRecv,
			Pmtu:          &tcpinfo.Pmtu,
			RcvSsthresh:   &tcpinfo.RcvSsthresh,
			Rtt:           &tcpinfo.Rtt,
			RttVar:        &tcpinfo.Rttvar,
			SndSsthresh:   &tcpinfo.SndSsthresh,
			SndCwnd:       &tcpinfo.SndCwnd,
			AdvMss:        &tcpinfo.AdvMss,
			Reordering:    &tcpinfo.Reordering,
			RcvRtt:        &tcpinfo.RcvRtt,
			RcvSpace:      &tcpinfo.RcvSpace,
			TotalRetrans:  &tcpinfo.TotalRetrans,
			PacingRate:    &tcpinfo.PacingRate,
			MaxPacingRate: &tcpinfo.MaxPacingRate,
			BytesAcked:    &tcpinfo.BytesAcked,
			BytesReceived: &tcpinfo.BytesReceived,
			SegsOut:       &tcpinfo.SegsOut,
			SegsIn:        &tcpinfo.SegsIn,
			NotSentBytes:  &tcpinfo.NotSentBytes,
			MinRtt:        &tcpinfo.MinRtt,
			DataSegsIn:    &tcpinfo.DataSegsIn,
			DataSegsOut:   &tcpinfo.DataSegsOut,
			DeliveryRate:  &tcpinfo.DeliveryRate,
			BusyTime:      &tcpinfo.BusyTime,
			RwndLimited:   &tcpinfo.RwndLimited,
			SndbufLimited: &tcpinfo.SndbufLimited,
			// 5+ kernel
			// Delivered:     &tcpinfo.Delivered,
			// DeliveredCe:   &tcpinfo.DeliveredCe,
			// BytesSent:     &tcpinfo.BytesSent,
			// BytesRetrans:  &tcpinfo.BytesRetrans,
			// DsackDups:     &tcpinfo.DsackDups,
			// ReordSeen:     &tcpinfo.ReordSeen,
			// RcvOoopack:    &tcpinfo.RcvOoopack,
			// SndWnd:        &tcpinfo.SndWnd,
		},
		CongestionAlgorithmEnum: &congestionAlgorithmEnum,
		// TypeOfService:           &typeofserviceu32,
		// TrafficClass:            &trafficclassu32,
		SkMemInfo: &xtcppb.SkMemInfo{
			RmemAlloc:  &skmeminfo.RmemAlloc,
			RcvBuf:     &skmeminfo.RcvBuf,
			WmemAlloc:  &skmeminfo.WmemAlloc,
			SndBuf:     &skmeminfo.SndBuf,
			FwdAlloc:   &skmeminfo.FwdAlloc,
			WmemQueued: &skmeminfo.WmemQueued,
			Optmem:     &skmeminfo.Optmem,
			Backlog:    &skmeminfo.Backlog,
			Drops:      &skmeminfo.Drops,
		},
		// ShutdownState: &shutdownstateu32,
		// BbrInfo: &xtcppb.BbrInfo{
		// 	BwLo:       &bbrinfo.BwLo,
		// 	BwHi:       &bbrinfo.BwHi,
		// 	MinRtt:     &bbrinfo.MinRtt,
		// 	PacingGain: &bbrinfo.PacingGain,
		// 	CwndGain:   &bbrinfo.CwndGain,
		// },
		// ClassId: classID,
	}

	// Add BBR info struct if the congestion algorithm is BBR
	if congestionAlgorithmEnum == xtcppb.XtcpRecord_BBR1 {
		XtcpRecord.BbrInfo = &xtcppb.BbrInfo{
			BwLo:       &bbrinfo.BwLo,
			BwHi:       &bbrinfo.BwHi,
			MinRtt:     &bbrinfo.MinRtt,
			PacingGain: &bbrinfo.PacingGain,
			CwndGain:   &bbrinfo.CwndGain,
		}
	}

	// Only add these if they are non-zero
	// Also have to do type conversion if non-zero
	if *typeOfService != 0 {
		var typeofserviceu32 = uint32(*typeOfService)
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "typeofserviceu32:", typeofserviceu32)
		}
		XtcpRecord.TypeOfService = &typeofserviceu32
	}
	if *trafficClass != 0 {
		var trafficclassu32 = uint32(*trafficClass)
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "trafficclassu32:", trafficclassu32)
		}
		XtcpRecord.TrafficClass = &trafficclassu32
	}
	if *shutdownState != 0 {
		var shutdownstateu32 = uint32(*shutdownState)
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "shutdownstateu32:", shutdownstateu32)
		}
		XtcpRecord.ShutdownState = &shutdownstateu32
	}
	if *classID != 0 {
		var classID32 = uint32(*classID)
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "classId32:", classID32)
		}
		XtcpRecord.ClassId = &classID32
	}

	if report == true {
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.Hostname:", *XtcpRecord.Hostname)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.EpochTime.Sec:", *XtcpRecord.EpochTime.Sec)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.EpochTime.Nsec:", *XtcpRecord.EpochTime.Nsec)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.InetDiagMsg.Family:", *XtcpRecord.InetDiagMsg.Family)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.InetDiagMsg.Inode:", *XtcpRecord.InetDiagMsg.Inode)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.Rtt:", *XtcpRecord.TcpInfo.Rtt)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.RttVar:", *XtcpRecord.TcpInfo.RttVar)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.RcvRtt:", *XtcpRecord.TcpInfo.RcvRtt)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.MinRtt:", *XtcpRecord.TcpInfo.MinRtt)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.DeliveryRate:", *XtcpRecord.TcpInfo.DeliveryRate)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.TcpInfo.DeliveryRateAppLimited:", *XtcpRecord.TcpInfo.DeliveryRateAppLimited)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.BbrInfo.MinRtt:", *XtcpRecord.BbrInfo.MinRtt)
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.CongestionAlgorithmEnum:", *XtcpRecord.CongestionAlgorithmEnum)
		}
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "XtcpRecord.Hostname:", *XtcpRecord.Hostname, "\tXtcpRecord.EpochTime.Sec:", *XtcpRecord.EpochTime.Sec, "\tXtcpRecord.EpochTime.Nsec:", *XtcpRecord.EpochTime.Nsec, "\tXtcpRecord.InetDiagMsg.Family:", *XtcpRecord.InetDiagMsg.Family, "\tXtcpRecord.InetDiagMsg.Inode:", *XtcpRecord.InetDiagMsg.Inode, "\tXtcpRecord.TcpInfo.Rtt:", *XtcpRecord.TcpInfo.Rtt, "\tXtcpRecord.TcpInfo.RttVar:", *XtcpRecord.TcpInfo.RttVar, "\tXtcpRecord.TcpInfo.RcvRtt:", *XtcpRecord.TcpInfo.RcvRtt, "\tXtcpRecord.TcpInfo.MinRtt:", *XtcpRecord.TcpInfo.MinRtt, "\tXtcpRecord.TcpInfo.DeliveryRate:", *XtcpRecord.TcpInfo.DeliveryRate, "\tXtcpRecord.BbrInfo.MinRtt:", *XtcpRecord.BbrInfo.MinRtt, "\tXtcpRecord.CongestionAlgorithmEnum:", *XtcpRecord.CongestionAlgorithmEnum)
		}
	}
	return XtcpRecord
}

func processNetlinkAttributes(id int, af *uint8, inetdiagMsgReader *bytes.Reader, meminfo *inetdiag.MemInfo, tcpinfo *inetdiag.TCPInfo415, sndWscale *uint32, rcvWscale *uint32, congestionAlgorithm *string, typeOfService *uint8, trafficClass *uint8, skmeminfo *inetdiag.SkMemInfo, shutdownState *uint8, bbrinfo *inetdiag.BBRInfo, classID *uint32, mark *uint32, deliveryRateAppLimited *uint32, fastOpenClientFail *uint32) (inetdiagMsgComplete bool, bytesRead int, padBufferSize int) {

	var nlattr inetdiag.Nlattr
	var attribuesCount int
	var netlinkAttributeDataLength int

	// Now the Netlink TCP diag attributes for this socket.  We should get 7 of these, based on what we requested.
	for attribuesCount = 0; !inetdiagMsgComplete; attribuesCount++ {

		// type Nlattr struct {
		// 	NlaLen  uint16
		// 	NlaType uint16
		// }
		if debugLevel > 1000 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\t-------------------------\tprocessNetlinkAttributes")
		}
		var err error
		err = binary.Read(inetdiagMsgReader, binary.LittleEndian, &nlattr)
		if err != nil {
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tbinary.Read nlattr failed:", err)
			}
			if err == io.EOF {
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tnlattr EOF")
				}
				inetdiagMsgComplete = true
				break
			}
		}
		bytesRead += binary.Size(nlattr)

		netlinkAttributeDataLength = int(nlattr.NlaLen) - binary.Size(nlattr)

		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\tnlattr.NlaType:", nlattr.NlaType, "\tnlattr.NlaLen:", nlattr.NlaLen, "\tattribuesCount:", attribuesCount)
		}

		// The following giant switch is based on constants from an enum defined as follows
		// This code might/or-not be improved by defining this list as constants, but for now, just using comments
		// If the enum changes, this is going to break anyway, so unless we cleverly pulled in the enum somehow
		// we're going ot have to touch this code, so commments are ok for now.

		// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L133
		// INET_DIAG_NONE 0
		// INET_DIAG_MEMINFO 1
		// INET_DIAG_INFO 2
		// INET_DIAG_VEGASINFO 3
		// INET_DIAG_CONG 4
		// INET_DIAG_TOS 5
		// INET_DIAG_TCLASS 6
		// INET_DIAG_SKMEMINFO 7
		// INET_DIAG_SHUTDOWN 8
		// INET_DIAG_DCNFO 9
		// INET_DIAG_PROTOCOL 10
		// INET_DIAG_SKV6ONLY 11
		// INET_DIAG_LOCALS 12
		// INET_DIAG_PEERS 13
		// INET_DIAG_PAD 14
		// INET_DIAG_MARK 15
		// INET_DIAG_BBRINFO 16
		// INET_DIAG_CLASS_ID 17
		// INET_DIAG_MD5SIG 18

		var attributesBytesRead int //this variable is used to allow the padding, if the structs in the kernel grow, or for 32bit alignment
		switch nlattr.NlaType {
		//INET_DIAG_NONE 0
		// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L132
		case 0:
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_NONE")
				fmt.Println("inetdiager:", id, "\taf:", *af, "\texit the NetLink attributes loops!! attribuesCount:", attribuesCount)
			}
			inetdiagMsgComplete = true
			break
		//INET_DIAG_MEMINFO
		// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L174
		case 1:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_MEMINFO", inetdiagMsgReader, meminfo, netlinkAttributeDataLength, af)
			break
		//INET_DIAG_INFO -- <<<--- THIS IS THE BIG IMPORTANT ONE
		// The payload associated with this attribute is specific to the address family.  For TCP sockets, it is an object of type struct tcp_info.
		case 2:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_INFO", inetdiagMsgReader, tcpinfo, netlinkAttributeDataLength, af)
			*sndWscale = uint32(tcpinfo.ScaleTemp >> 4)   // 4 bits of the left
			*rcvWscale = uint32(tcpinfo.ScaleTemp & 0x0F) // the 4 bits to the right
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_INFO\t*sndWscale:", *sndWscale, "\t*rcvWscale:", *rcvWscale)
			}
			*deliveryRateAppLimited = uint32(tcpinfo.FlagsTemp & 0x1) // right most bit
			*fastOpenClientFail = uint32(tcpinfo.FlagsTemp >> 1)      // 2nd bit from the right
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_INFO\t*deliveryRateAppLimited:", *deliveryRateAppLimited, "\t*fastOpenClientFail:", *fastOpenClientFail)
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_INFO\ttcpinfo:", tcpinfo)
			}
			// TODO add delivery rate app limited and fast open here
			break
		//INET_DIAG_VEGASINFO
		case 3:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_SKV6ONLY", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_CONG
		case 4:
			//Unlike most of the attributes, the congestion algorithm is variable length null terminated array of chars (C string)
			congestionAlgorithmBuffer := make([]byte, (int)(nlattr.NlaLen)-binary.Size(nlattr))
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_CONG", inetdiagMsgReader, &congestionAlgorithmBuffer, netlinkAttributeDataLength, af)
			if bytesRead > 0 {
				// storing only the first three chars of the string, so the map lookup works in buildProto
				*congestionAlgorithm = string(congestionAlgorithmBuffer[:len(congestionAlgorithmBuffer)])[:3]
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tstring(congestionAlgorithmBuffer)[:3]:", *congestionAlgorithm)
				}
			}
			break
		//INET_DIAG_TOS
		case 5:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_TOS", inetdiagMsgReader, typeOfService, netlinkAttributeDataLength, af)
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_TOS\t*typeOfService:", *typeOfService)
			}
			break
		//INET_DIAG_TCLASS
		// The payload associated with this attribute is a __u8  value which is the TClass of the socket.  IPv6 sockets
		// only.  For LISTEN and CLOSE sockets, this is followed by INET_DIAG_SKV6ONLY attribute with associated __u8
		// payload value meaning whether the socket is IPv6-only or not.
		case 6:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "tINET_DIAG_TCLASS", inetdiagMsgReader, trafficClass, netlinkAttributeDataLength, af)
			break
		//INET_DIAG_SKMEMINFO
		// https://github.com/torvalds/linux/blob/a811c1fa0a02c062555b54651065899437bacdbe/net/core/sock.c#L3226
		case 7:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_SKMEMINFO", inetdiagMsgReader, skmeminfo, netlinkAttributeDataLength, af)
			break
		//UNIX_DIAG_SHUTDOWN
		// The payload associated with this attribute is __u8 value which represents bits of shutdown(2) state.
		case 8:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_SHUTDOWN", inetdiagMsgReader, shutdownState, netlinkAttributeDataLength, af)
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_SHUTDOWN\t*shutdownState:", *shutdownState)
			}
			break
		//--- NOT INET_DIAG_DCINFO - no body uses this UDP protocol
		case 9:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_DCINFO", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_PROTOCOL
		case 10:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_PROTOCOL", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_SKV6ONLY
		// TODO per the comment in INET_DIAG_TCLASS above, need to handle this case for IPv6 LISTEN and CLOSE sockets
		case 11:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_SKV6ONLY", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_LOCALS
		case 12:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_LOCALS", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_PEERS
		case 13:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_PEERS", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_PAD
		case 14:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_PAD", "\tERROR!!  TODO Fix me")
			}
			break
		//INET_DIAG_MARK
		case 15:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_MARK", inetdiagMsgReader, mark, netlinkAttributeDataLength, af)
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_MARK\t*mark:", *mark)
			}
			break
		//INET_DIAG_BBRINFO
		case 16:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_BBRINFO", inetdiagMsgReader, bbrinfo, netlinkAttributeDataLength, af)
			break
		// INET_DIAG_CLASS_ID
		// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/linux/inet_diag.h#L74
		// + nla_total_size(4); /* INET_DIAG_CLASS_ID *
		case 17:
			inetdiagMsgComplete, attributesBytesRead = binaryReadWithErrorHandling(id, "INET_DIAG_CLASS_ID", inetdiagMsgReader, classID, netlinkAttributeDataLength, af)
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_TOS\t*typeOfService:", *typeOfService)
			}
			break	
		case 18:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tINET_DIAG_LOCALS", "\tERROR!!  TODO Fix me")
			}
			break	
		default:
			inetdiagMsgComplete, attributesBytesRead = notDecodingThisAttributeTypeYet()
			if debugLevel > 10 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tnlattr.NlaType default??", nlattr.NlaType, "\tERROR!!  TODO Fix me")
			}
			break
		}
		//switch nlattr.NlaType {

		// -------------------------------------------------
		// Deal with alignment & padding
		//
		// Calculate any required padding and alignment
		// This is in case the amount of data we read in is less than is actually in the record
		//   e.g. Current kernel struct for tcp_info is bigger than the struct defined in this program
		// Also we pad for 32bit alignment, which is mostly for the 8bit fields, which end up padding by 3 bytes.
		var padSize int
		if (attributesBytesRead + binary.Size(nlattr)) < (int)(nlattr.NlaLen) {
			padSize = (int)(nlattr.NlaLen) - attributesBytesRead - binary.Size(nlattr)
			if debugLevel > 100 {
				if padSize > 0 {
					//fmt.Println("inetdiager:", id, "\taf:", *af, "\tnlattr.NlaLen:", nlattr.NlaLen, "\t- bytesRead:", bytesRead, "\t+binary.Size(nlattr):", binary.Size(nlattr))
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tpadSize:", padSize)
				}
			}
		}
		var padAlignSize int
		if (int)(nlattr.NlaLen)%4 > 0 {
			padAlignSize = 4 - (int)(nlattr.NlaLen)%4 //alignment to 4 bytes = 32 bits
		}
		if debugLevel > 100 {
			if padAlignSize > 0 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tpadAlignSize:", padAlignSize)
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tpadSize+padAlignSize:", padSize+padAlignSize)
			}
		}
		if padSize+padAlignSize > 0 {
			// possibly it would make sense to have some precreated sizes? future improvement option
			padBuffer := make([]byte, padSize+padAlignSize)
			err = binary.Read(inetdiagMsgReader, binary.LittleEndian, &padBuffer)
			if err != nil {
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tbinary.Read padBuffer failed:", err)
				}
				if err == io.EOF {
					if debugLevel > 100 {
						fmt.Println("inetdiager:", id, "\taf:", *af, "\tpadBuffer EOF")
					}
				}
				inetdiagMsgComplete = true
				break
			}
			//let's take a peak at what's in the pad buffer
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tpadBuffer:", padBuffer)
			}
			padBufferSize += binary.Size(padBuffer)
			bytesRead += attributesBytesRead + binary.Size(padBuffer)

		}
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\tprocessNetlinkAttributes\tinetdiagMsgComplete:", inetdiagMsgComplete, "\tbytesRead:", bytesRead)
		}
	}
	//for attribuesCount := 0; !inetdiagMsgComplete; attribuesCount++ {

	return inetdiagMsgComplete, bytesRead, padBufferSize
}

func sendToNSQ(topic string, message []byte, nsqServer string) error {
	config := nsq.NewConfig()
	producer, _ := nsq.NewProducer(nsqServer, config)

	err := producer.Publish(topic, message)
	if err != nil {
		producer.Stop()
		return err
	}
	producer.Stop()
	return nil
}

// Inetdiager is the worker which recieves the Inetdiag messages from the netlinker
// This functino does the heavy lifting in terms of parsing the inetdiag messages
// currently we don't need the netlinkerDone channel, but we will once this function passes downstream
func Inetdiager(id int, af *uint8, in <-chan netlinker.TimeSpecandInetDiagMessage, wg *sync.WaitGroup, hostname string, cliFlags cliflags.CliFlags, inetdiagerStaterCh chan<- inetdiagerstater.InetdiagerStatsWrapper) {

	//defer close(out)
	defer wg.Done()

	var inetdiagMsg inetdiag.InetDiagMsg
	//var nlattr inetdiag.Nlattr

	var inetdiagMsgCount int
	var inetdiagMsgInSize int
	var inetdiagMsgBytesRemaining int
	var inetdiagMsgBytesRead int
	var inetdiagMsgInSizeTotal int
	var inetdiagMsgBytesReadTotal int
	var padBufferTotal int

	var udpWritesTotal int
	var udpBytesWrittenTotal int
	var udpErrorsTotal int

	var statsBlocked int

	var currentStats inetdiagerstater.InetdiagerStatsWrapper

	var meminfo inetdiag.MemInfo
	var skmeminfo inetdiag.SkMemInfo
	var tcpinfo inetdiag.TCPInfo415
	var bbrinfo inetdiag.BBRInfo
	var shutdownState uint8
	var typeOfService uint8
	var trafficClass uint8
	var classID uint32
	var congestionAlgorithm string
	var mark uint32 // not really sure what this is actually
	var sourceIP net.IP
	var destinationIP net.IP
	var sourceIPbytes []byte
	var destinationIPbytes []byte
	var sndWscale uint32
	var rcvWscale uint32
	var deliveryRateAppLimited uint32
	var fastOpenClientFail uint32
	// TODO add delivery rate app limited and fast open here

	//-----------------------------------------------
	// This is the timer for when the inetdiager will send summary stats to the inetdiagerStater (ratio of pollingFrequency)
	statsTicker := time.NewTicker(time.Duration(float64(*cliFlags.PollingFrequency) * *cliFlags.InetdiagerStatsRatio))

	//-----------------------------------------------
	// Create UDP socket to send protobufs over
	udpConn, dialerr := net.Dial("udp", *cliFlags.UDPSendDest)
	if dialerr != nil {
		if debugLevel > 10 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\tnet.Dial(\"udp\", ", *cliFlags.UDPSendDest, ") error:", dialerr)
		}
	}
	defer udpConn.Close()

	// This is range over the channel
	// (Remember that when the channel gets closed, this loops complete, and so this inetdiager will close
	// This is how the shutdownWorkers closes these workers. )
	for timeSpecandInetDiagMessage := range in {

		inetdiagMsgInSize = len(timeSpecandInetDiagMessage.InetDiagMessage)
		inetdiagMsgInSizeTotal += inetdiagMsgInSize
		inetdiagMsgBytesRemaining = inetdiagMsgInSize
		inetdiagMsgBytesRead = 0
		inetdiagMsgReader := bytes.NewReader(timeSpecandInetDiagMessage.InetDiagMessage)
		if debugLevel > 100 {
			fmt.Println("inetdiager:", id, "\taf:", *af, "\tinetdiagMsg = <-in\tinetdiagMsgInSize:", inetdiagMsgInSize)
		}

		// Send stats to the inetdiagerStater if the reporting duration has elapsed
		// This basically dones a non-blocking read of the timer channel
		// If the timer is up, then it sends summary stats over the channel
		// Otherwise, it just rolls on though doing nothing.
		select {
		case _ = <-statsTicker.C:
			currentStats = inetdiagerstater.InetdiagerStatsWrapper{
				Af: *af,
				ID: id,
				Stats: inetdiagerstater.InetdiagerStats{
					InetdiagMsgInSizeTotal:    inetdiagMsgInSizeTotal,
					InetdiagMsgCount:          inetdiagMsgCount,
					InetdiagMsgBytesReadTotal: inetdiagMsgBytesReadTotal,
					PadBufferTotal:            padBufferTotal,
					UDPWritesTotal:            udpWritesTotal,
					UDPBytesWrittenTotal:      udpBytesWrittenTotal,
					UDPErrorsTotal:            udpErrorsTotal,
					StatsBlocked:              statsBlocked,
				},
			}
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tTick!\t", currentStats)
			}
			// this select is to track if the stats channel is blocking
			select {
			case inetdiagerStaterCh <- currentStats:
			default:
				// increment blocked couner, and then do a blocking send
				// ( we're not tracking duration cos this is only stats, and duration calculations are kind of expensive )
				statsBlocked++
				inetdiagerStaterCh <- currentStats //block
			}
		default:
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tNo tick!")
			}
		}

		for inetdiagMsgComplete := false; !inetdiagMsgComplete && inetdiagMsgBytesRemaining > 0; {
			err := binary.Read(inetdiagMsgReader, binary.LittleEndian, &inetdiagMsg)
			if err != nil {
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tbinary.Read inetdiagMsg failed:", err)
				}
				if err == io.EOF {
					if debugLevel > 100 {
						fmt.Println("readNetLinkMessagesWorker:", id, "\tinetdiagMsgReader EOF")
					}
					inetdiagMsgComplete = true
					break
				}
			}

			inetdiagMsgBytesRead += binary.Size(inetdiagMsg)
			inetdiagMsgBytesRemaining -= binary.Size(inetdiagMsg)
			inetdiagMsgBytesReadTotal += binary.Size(inetdiagMsg)

			// TODO Could switch this out to binary.Read binary.BigEndian
			// this is a to flip the endianess of the ports
			// the alternative to doing this would be to have x3 binary.Reads
			// this is because the struct is specifically in BigEndian
			// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L13
			// struct inet_diag_sockid {
			// 	__be16	idiag_sport;
			// 	__be16	idiag_dport;
			inetdiagMsg.SocketID.SourcePort = SwapUint16(inetdiagMsg.SocketID.SourcePort)
			inetdiagMsg.SocketID.DestinationPort = SwapUint16(inetdiagMsg.SocketID.DestinationPort)

			// inetdiagMsg src/dst addresses encode IPv4 addresses in the first 4 bytes
			// of a 16 byte array. Unfortunately the net.IP package expects IPv4 addresses
			// to be encoded in the last 4 bytes of a 16 byte array. As a result, we must
			// pass only 4 bytes to net.IP for AF_INET.
			// Doing conversion to golang net.IP() type to allow printing here, but we actually use the bytes to put into the protobuf
			// Please note that net.IP is mostly for Println, as the bytes version is used to populate the protobuf
			switch inetdiagMsg.Family {
			case syscall.AF_INET6:
				sourceIP = net.IP(inetdiagMsg.SocketID.Source[:])
				destinationIP = net.IP(inetdiagMsg.SocketID.Destination[:])
				sourceIPbytes = inetdiagMsg.SocketID.Source[:]
				destinationIPbytes = inetdiagMsg.SocketID.Destination[:]
			case syscall.AF_INET:
				sourceIP = net.IP(inetdiagMsg.SocketID.Source[:4])
				destinationIP = net.IP(inetdiagMsg.SocketID.Destination[:4])
				sourceIPbytes = inetdiagMsg.SocketID.Source[:4]
				destinationIPbytes = inetdiagMsg.SocketID.Destination[:4]
			default:
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tUnknown address family")
				}
			}

			var bytesRead int
			var padBufferSize int
			inetdiagMsgComplete, bytesRead, padBufferSize = processNetlinkAttributes(id, af, inetdiagMsgReader, &meminfo, &tcpinfo, &sndWscale, &rcvWscale, &congestionAlgorithm, &typeOfService, &trafficClass, &skmeminfo, &shutdownState, &bbrinfo, &classID, &mark, &deliveryRateAppLimited, &fastOpenClientFail)
			inetdiagMsgBytesRead += bytesRead
			inetdiagMsgBytesRemaining -= bytesRead
			inetdiagMsgBytesReadTotal += bytesRead
			padBufferTotal += padBufferSize

			// cli reporting frequency based on constant, as a variable to be able to pass to buildProto
			if debugLevel > 100 {
				fmt.Println("inetdiager:", id, "\taf:", *af, "\tinetdiagMsgCount:", inetdiagMsgCount, "\t*cliFlags.inetdiagerReportModulus:", *cliFlags.InetdiagerReportModulus, "\tmodulus:", inetdiagMsgCount%(*cliFlags.InetdiagerReportModulus))
			}

			if *cliFlags.InetdiagerReportModulus == 1 || inetdiagMsgCount%*cliFlags.InetdiagerReportModulus == 1 {

				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tinetdiagMsgCount:", inetdiagMsgCount, "\tinetdiagMsgBytesReadTotal(M):", inetdiagMsgBytesReadTotal/10^6)
				}
				if debugLevel > 1000 {
					printInetDiagMsg(id, af, inetdiagMsg, sourceIP, destinationIP)
				}

				var XtcpRecord *xtcppb.XtcpRecord
				XtcpRecord = buildProto(id, af, &timeSpecandInetDiagMessage.TimeSpec, &hostname, &inetdiagMsg, sourceIPbytes, destinationIPbytes, &meminfo, &tcpinfo, &congestionAlgorithm, &shutdownState, &typeOfService, &trafficClass, &skmeminfo, &bbrinfo, &classID, &sndWscale, &rcvWscale, true, &deliveryRateAppLimited, &fastOpenClientFail)

				// https://pkg.go.dev/google.golang.org/protobuf/proto?tab=doc#Marshal
				XtcpRecordBinary, marshalErr := proto.Marshal(XtcpRecord)
				if marshalErr != nil {
					fmt.Println("proto.Marshal(XtcpRecord) error: ", marshalErr)
				}
				if debugLevel > 10000 {
					fmt.Println(XtcpRecordBinary)
				}

				// Send to NSQ
				if *cliFlags.NSQ != "" {
					err := sendToNSQ("xtcp", XtcpRecordBinary, *cliFlags.NSQ)
					if err != nil {
						fmt.Println("sendToNSQ(XtcpRecordBinary) error:", err)
					}
				}
				// Write the protobuf to the UDP socket
				udpBytesWritten, udpWriteErr := udpConn.Write(XtcpRecordBinary)
				if udpWriteErr != nil {
					if debugLevel > 100 {
						fmt.Println("udpConn.Write(XtcpRecordBinary) error: ", udpWriteErr)
					}
					udpErrorsTotal++
				}
				udpWritesTotal++
				udpBytesWrittenTotal += udpBytesWritten
				if debugLevel > 100 {
					fmt.Println("inetdiager:", id, "\taf:", *af, "\tudpConn.Write bytes written:", udpBytesWritten, "\tudpWritesTotal:", udpWritesTotal, "\tudpBytesWrittenTotal:", udpBytesWrittenTotal)
				}

				if debugLevel > 10000 {
					XtcpRecordJSON := protojson.Format(XtcpRecord)
					if err != nil {
						fmt.Println("protojson.Format(XtcpRecord) error: ", err)
					}
					fmt.Println(XtcpRecordJSON)
				}
			}
			inetdiagMsgCount++
		}
		//for inetdiagMsgComplete := false; !inetdiagMsgComplete && inetdiagMsgBytesRemaining > 0; {
	}
	//for {
	if debugLevel > 100 {
		fmt.Println("inetdiager:", id, "\taf:", *af, "\tclose")
	}
}
