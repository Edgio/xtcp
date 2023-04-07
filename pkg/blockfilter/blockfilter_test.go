// Tests for blockfilter
package blockfilter_test

import (
	"net"
	"testing"

	"github.com/Edgio/xtcp/pkg/blockfilter"
)

func TestLoadGroupNetworks(t *testing.T) {
	var tests = []struct {
		file     string
		group    string
		expected *blockfilter.NetBlocks
	}{
		{"badjson", "aaa", nil},
		{"testdata/test.json", "aab", nil},
	}

	for _, test := range tests {
		output := blockfilter.LoadGroupNetworks(test.file, test.group)
		if output != test.expected {
			t.Error("Test Failed: input {} {}, expected {}, recieved {}", test.file, test.group, test.expected, output)
		}
	}
}

func TestIsFilterEmpty(t *testing.T) {
	// Test to see how it behaves if we give it a nil lookup blockl
	var tests = []struct {
		ip       net.IP
		expected bool
	}{
		{net.IP{127, 0, 0, 1}, false}, // localhost
		{net.IP{128, 0, 0, 1}, false}, // Random block
	}
	for _, test := range tests {
		output := blockfilter.IsFilter(test.ip, nil)
		if output != test.expected {
			t.Error("Test Failed: input {}, expected {}, recieved {}", test.ip, test.expected, output)
		}
	}
}

func TestIsFilter(t *testing.T) {
	// Load up the sample json for comoparison
	test_blocks := blockfilter.LoadGroupNetworks("testdata/test.json", "aaa")

	var tests = []struct {
		ip       net.IP
		expected bool
	}{
		{net.IP{127, 0, 0, 1}, false},      // localhost
		{net.IP{128, 0, 0, 1}, false},      // Random block
		{net.IP{192, 229, 217, 10}, true},  // Included v4
		{net.IP{192, 229, 218, 10}, false}, // off-by-1 v4
		{net.IP{10, 30, 31, 11}, false},    // Another v4, this will be excluded because 1918
		{net.IP{0x26, 0x06, 0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, true},  //Included v6
		{net.IP{0x26, 0x06, 0x29, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, false}, // Off by 1 v6
	}

	for _, test := range tests {
		output := blockfilter.IsFilter(test.ip, test_blocks)
		if output != test.expected {
			t.Error("Test Failed: input {}, expected {}, recieved {}", test.ip, test.expected, output)
		}
	}

}

func TestRangeToCIDR(t *testing.T) {
	var tests = []struct {
		start    net.IP
		end      net.IP
		expected string
	}{
		{net.IP{192, 0, 0, 1}, net.IP{192, 0, 0, 1}, "192.0.0.1/32"},
		{net.IP{192, 0, 0, 1}, net.IP{192, 0, 0, 2}, "192.0.0.0/30"},
		{net.IP{192, 0, 0, 1}, net.IP{192, 0, 1, 0}, "192.0.0.0/23"},
		{net.IP{0x26, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			net.IP{0x26, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			"2606::1/128"},
		{net.IP{0x26, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			net.IP{0x26, 0x06, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			"2606::/48"},
	}

	for _, test := range tests {
		output := blockfilter.RangeToCIDR(test.start, test.end)
		if output.String() != test.expected {
			t.Error("Test Failed: input {} {}, expected {}, recieved {}", test.start, test.end, test.expected, output)
		}
	}

}
