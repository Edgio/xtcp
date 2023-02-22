package netlinker_test

// TODO Write more tests!!!

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/Edgio/xtcp/pkg/netlinker"
	"golang.org/x/sys/unix"
)

const (
	debugLevel int = 11
)

// TestCheckNetlinkMessageTypes does some basic checks of the validity of the function, including a single negative test case
func TestCheckNetlinkMessageTypes(t *testing.T) {
	var tests = []struct {
		id                 int
		af                 uint8
		Type               uint16
		netlinkMsgComplete bool
		netlinkMsgDone     bool
		errorCount         int
	}{
		{1, 2, unix.NLMSG_NOOP, true, false, 1}, // test 0
		{1, 2, unix.NLMSG_DONE, true, true, 0},
		{1, 2, unix.NLMSG_ERROR, true, false, 1},
		{1, 2, unix.NLMSG_OVERRUN, true, false, 1},
		{1, 2, uint16(999), false, false, 0}, // test 4
		{1, 2, uint16(999), false, false, 1}, // test 5 - negative test case
	}

	for i, test := range tests {
		netlinkMsgComplete, netlinkMsgDone, errorCount := netlinker.CheckNetlinkMessageType(test.id, &test.af, test.Type)
		if debugLevel > 100 {
			out := fmt.Sprintf("netlinkMsgComplete Test: id %d, af %d, Type %s, expected netlinkMsgComplete %s!=%s, netlinkMsgDone %s!=%s, errorCount %d!=%d",
				test.id,
				test.af,
				fmt.Sprint(test.Type),
				strconv.FormatBool(test.netlinkMsgComplete),
				strconv.FormatBool(netlinkMsgComplete),
				strconv.FormatBool(test.netlinkMsgDone),
				strconv.FormatBool(netlinkMsgDone),
				test.errorCount,
				errorCount,
			)
			fmt.Println(out)
		}
		if netlinkMsgComplete != test.netlinkMsgComplete || netlinkMsgDone != test.netlinkMsgDone || errorCount != test.errorCount {
			// test 5 is the negative case
			if i != 5 {
				t.Errorf("netlinkMsgComplete Test Failed: id %d, af %d, Type %s, expected netlinkMsgComplete %s!=%s, netlinkMsgDone %s!=%s, errorCount %d!=%d",
					test.id,
					test.af,
					fmt.Sprint(test.Type),
					strconv.FormatBool(test.netlinkMsgComplete),
					strconv.FormatBool(netlinkMsgComplete),
					strconv.FormatBool(test.netlinkMsgDone),
					strconv.FormatBool(netlinkMsgDone),
					test.errorCount,
					errorCount,
				)
			}
		}
	}
}
