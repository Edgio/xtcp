// Package xtcpstater_test
// These tests actually used a precreated "systemctl status xtcp" output file "fake_systemctl_status"
package xtcpstater_test

import (
	"fmt"
	"testing"

	"github.com/Edgio/xtcp/pkg/xtcpstater"
)

const (
	debugLevel int = 11
)

// TestgetSystemCtlStatus performs a basic check that xtcpstater.GetSystemCtlStatus works
func TestGetSystemCtlStatus(t *testing.T) {
	var tests = []struct {
		filename string
		pid      string
		tasks    string
	}{
		{"./testdata/fake_systemctl_status", "11879", "12"},   // test 0 - TODO add more test files
		{"./testdata/fake_systemctl_statusG", "11879", "12"},  // test 1 - Added a G to test the G code
		{"./testdata/fake_systemctl_statusG", "1187", "12"},   // test 2 - negative pid
		{"./testdata/fake_systemctl_statusG", "11879", "120"}, // test 3 - negative tasks
		{"./testdata/fake_systemctl_statusG", "11879", "12"},  // test 4 - negative memory
	}

	for i, test := range tests {

		if debugLevel > 10 {
			fmt.Println("TestGetSystemCtlStatus test:", i, "\t", test)
		}

		pid, tasks := xtcpstater.GetSystemCtlStatus("/usr/bin/systemctl", true, test.filename)
		switch i {
		case 2:
			if pid != test.pid {
				if tasks != test.tasks {
					t.Errorf("GetSystemCtlStatus Test Failed: pid %s!=%s, tasks %s!=%s",
						test.pid,
						pid,
						test.tasks,
						tasks,
					)
				}
				if debugLevel > 100 {
					fmt.Println("test:", i, " success")
				}
			}
		case 3:
			if tasks != test.tasks {
				if pid != test.pid {
					t.Errorf("GetSystemCtlStatus Test Failed: pid %s!=%s, tasks %s!=%s",
						test.pid,
						pid,
						test.tasks,
						tasks,
					)
				}
				if debugLevel > 100 {
					fmt.Println("test:", i, " success")
				}
			}
		default:
			if pid != test.pid || tasks != test.tasks {
				t.Errorf("GetSystemCtlStatus Test Failed: pid %s!=%s, tasks %s!=%s",
					test.pid,
					pid,
					test.tasks,
					tasks,
				)
			}
		}

	}
}

func benchmarkGetSystemCtlStatus(filename string, b *testing.B) {

	// run the GetSystemCtlStatus function b.N times
	for n := 0; n < b.N; n++ {
		xtcpstater.GetSystemCtlStatus("/usr/bin/systemctl", true, filename)
	}
}

func BenchmarkGetSystemCtlStatus(b *testing.B) {

	var tests = []struct {
		filename string
	}{
		{"./testdata/fake_systemctl_status"},  // test 0
		{"./testdata/fake_systemctl_statusG"}, // test 1
		{"./testdata/fake_systemctl_statusG"}, // test 2
		{"./testdata/fake_systemctl_statusG"}, // test 3
		{"./testdata/fake_systemctl_statusG"}, // test 4
	}
	for i, test := range tests {

		if debugLevel > 10 {
			fmt.Println("BenchmarkGetSystemCtlStatus test:", i, "\t", test)
		}
		benchmarkGetSystemCtlStatus(test.filename, b)
	}
}

// TestGetPSStats uses some input files based on output from production to valdiate the parsing function works
func TestGetPSStats(t *testing.T) {
	var tests = []struct {
		filename string
		pcpu     string
		pmem     string
		rss      string
		sz       string
	}{
		{"./testdata/fake_ps1", "4.7", "0.0", "13748", "267972"}, // test 0 - TODO add more test files
		{"./testdata/fake_ps2", "4.7", "0.0", "13876", "267908"}, // test 1
		{"./testdata/fake_ps3", "5.0", "0.0", "14132", "286405"}, // test 2
	}
	// das@das-dell5580:~/go/src/github.com/Edgio/xtcp/pkg/xtcpstater$ cat fake_ps*
	// 4.7  0.0 13748 267972
	// 4.7  0.0 13876 267908
	// 5.0  0.0 14132 286405

	for i, test := range tests {

		if debugLevel > 10 {
			fmt.Println("TestGetPSStats test:", i, "\t", test)
		}

		pcpu, pmem, rss, sz := xtcpstater.GetPSStats("666", "/usr/bin/ps", true, test.filename) // 666 is just any PID

		if pcpu != test.pcpu || pmem != test.pmem || rss != test.rss || sz != test.sz {
			t.Errorf("GetPSStats Test Failed: pcpu %s!=%s, pmem %s!=%s, rss %s!=%s, sz %s!=%s",
				test.pcpu,
				pcpu,
				test.pmem,
				pmem,
				test.rss,
				rss,
				test.sz,
				sz,
			)
		}
	}
}

func benchmarkGetPSStats(filename string, b *testing.B) {

	// run the GetSystemCtlStatus function b.N times
	for n := 0; n < b.N; n++ {
		xtcpstater.GetPSStats("666", "/usr/bin/systemctl", true, filename)
	}
}

func BenchmarkGetPSStats(b *testing.B) {

	var tests = []struct {
		filename string
	}{
		{"./testdata/fake_ps1"}, // test 0
		{"./testdata/fake_ps2"}, // test 1
		{"./testdata/fake_ps3"}, // test 2
	}
	for i, test := range tests {

		if debugLevel > 10 {
			fmt.Println("BenchmarkGetPSStats test:", i, "\t", test)
		}
		benchmarkGetPSStats(test.filename, b)
	}
}
