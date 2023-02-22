package xtcpnl

import (
	"bytes"
	"encoding/binary"
	"syscall"
	"testing"

	"github.com/Edgio/xtcp/pkg/inetdiag" // kernel structs
)

func TestBuildNetlinkSockDiagRequest(t *testing.T) {
	var tests = []struct {
		addressFamily uint8
		make_size     int
		nlmsg_len     uint32
		nlmsg_seq     uint32
		nlmsg_pid     uint32
		idiag_ext     uint8
		idiag_states  uint8
	}{
		{2, 128, 72, 666, 0, 0xFF, 0},
		{2, 128, 72, 667, 0, 0xFF, 0},
	}

	// type NlMsgHdr struct {
	// 	Length   uint32
	// 	Type     uint16
	// 	Flags    uint16
	// 	Sequence uint32
	// 	Pid      uint32
	// }
	var netlinkMsgHeader inetdiag.NlMsgHdr

	for _, test := range tests {
		packetBytes := BuildNetlinkSockDiagRequest(&test.addressFamily, test.make_size, test.nlmsg_len, test.nlmsg_seq, test.nlmsg_pid, test.idiag_ext, test.idiag_states)
		if binary.Size(packetBytes) != test.make_size {
			t.Error("Test Failed: binary.Size(packetBytes) expected {}, recieved {} ", test.make_size, binary.Size(packetBytes))
		}
		packetBytesReader := bytes.NewReader(packetBytes)
		err := binary.Read(packetBytesReader, binary.LittleEndian, &netlinkMsgHeader)
		if err != nil {
			t.Error("Test Failed: binary.Read(packetBytesReader, binary.LittleEndian, &netlinkMsgHeader)")
		}
		if netlinkMsgHeader.Length != test.nlmsg_len {
			t.Error("Test Failed: netlinkMsgHeader.Length != test.nlmsg_len expected {}, recieved {} ", test.nlmsg_len, netlinkMsgHeader.Length)
		}
		if netlinkMsgHeader.Type != uint16(20) {
			t.Error("Test Failed: netlinkMsgHeader.Type != uint16(20), recieved {} ", netlinkMsgHeader.Type)
		}
		if netlinkMsgHeader.Flags != uint16(syscall.NLM_F_DUMP|syscall.NLM_F_REQUEST) {
			t.Error("Test Failed: netlinkMsgHeader.Flags != uint16(syscall.NLM_F_DUMP|syscall.NLM_F_REQUEST)), recieved {} ", netlinkMsgHeader.Flags)
		}
		if netlinkMsgHeader.Sequence != test.nlmsg_seq {
			t.Error("Test Failed: netlinkMsgHeader.Sequence != test.nlmsg_seq expected {}, recieved {} ", test.nlmsg_seq, netlinkMsgHeader.Sequence)
		}
		if netlinkMsgHeader.Pid != test.nlmsg_pid {
			t.Error("Test Failed: netlinkMsgHeader.Sequence != test.nlmsg_pid expected {}, recieved {} ", test.nlmsg_pid, netlinkMsgHeader.Pid)
		}
		// we could also check for lots of zeros after here, but this is a good start TODO
	}
}
