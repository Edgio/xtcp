package inetdiager_test

import (
	"testing"

	"github.com/Edgio/xtcp/pkg/inetdiager"
)

func TestSwapUint16(t *testing.T) {
	var tests = []struct {
		input    uint16
		expected uint16
	}{
		{0, 0},
		{1, 256},
		{16, 4096},
		{22, 5632},
		{27137, 362},
		{65520, 61695},
		{65535, 65535},
	}
	for _, test := range tests {
		if output := inetdiager.SwapUint16(test.input); output != test.expected {
			t.Error("Test Failed: input {}, expected {}, recieved {}", test.input, test.expected, output)
		}
	}
}
