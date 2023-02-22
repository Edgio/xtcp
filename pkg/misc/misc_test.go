package misc_test

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/Edgio/xtcp/pkg/misc"
)

// https://dave.cheney.net/2016/05/10/test-fixtures-in-go
// t.Log(wd)

const (
	debugLevel int = 11
)

// TestMaxLoopsOrForEver performs basic tests on misc.MaxLoopsOrForEver
func TestMaxLoopsOrForEver(t *testing.T) {
	var tests = []struct {
		pollingLoops int
		maxLoops     int
		expected     bool
	}{
		{10, 0, true}, // test 0
		{100, 0, true},
		{500, 0, true},
		{0, 1, true},
		{15, 16, true},
		{21, 22, true},
		{11, 10, false},
		{110, 100, false}, // test 7
		{10, 0, false},    // test 8 - negative
	}
	for i, test := range tests {

		if debugLevel > 100 {
			fmt.Println("test:\t", test)
		}

		if output := misc.MaxLoopsOrForEver(test.pollingLoops, test.maxLoops); output != test.expected {
			if i < 8 {
				t.Errorf("Faied test:%d\tpollingLoops:%d\tmaxLoops:%d\texpected:%s\tresult:%s",
					i,
					test.pollingLoops,
					test.maxLoops,
					strconv.FormatBool(test.expected),
					strconv.FormatBool(output),
				)
			}
		}
	}
}

// TestscanFile tests xtcpstater.scanFile
// The test is performed using the slower bufio.NewReader.
// We're basically just comparing if the two (2) techniques reach the same result
// The scanFile bufio.NewScanner technique, verse the test bufio.NewReader taken from here
// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go
// Please note the benchmarking code below doesn't seem to find much difference
func TestScanFile(t *testing.T) {

	filename := "./testdata/non_copy_write_text"
	scanFileLines := misc.ScanFile(filename)
	readFileLines := misc.ReadFile(filename)

	if !reflect.DeepEqual(scanFileLines, readFileLines) {

		t.Errorf("scanFile Test Failed: scanFileLines: %d readFileLines %d",
			len(scanFileLines),
			len(readFileLines))
	}
}

func BenchmarkScanFile(b *testing.B) {

	filename := "./testdata/non_copy_write_text"
	for n := 0; n < b.N; n++ {
		scanFileLines := misc.ScanFile(filename)
		if debugLevel > 100 {
			fmt.Println("len(scanFileLines):\t", len(scanFileLines))
		}
	}
}

func BenchmarkReadFile(b *testing.B) {

	filename := "./testdata/non_copy_write_text"
	for n := 0; n < b.N; n++ {
		readFileLines := misc.ReadFile(filename)
		if debugLevel > 100 {
			fmt.Println("len(readFileLines):\t", len(readFileLines))
		}
	}
}

func benchmarkFileN(n int, scanType string, b *testing.B) {

	// read in the file
	filename := "./testdata/non_copy_write_text"
	scanFileLines := misc.ScanFile(filename)

	// write out larger file (x100)
	filename = "./testdata/non_copy_write_text_new"
	//Create creates or truncates the named file. If the file already exists, it is truncated.
	f, err := os.Create(filename)
	if err != nil {
		fmt.Println(err)
		f.Close()
		return
	}

	for i := 0; i < n; i++ {
		for _, line := range scanFileLines {
			fmt.Fprintln(f, line)
			if err != nil {
				fmt.Println(err)
				return
			}
		}
	}
	err = f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

	// Benchmark timer RESET here!!!                       <--- Reset
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		if scanType == "scan" {
			scanFileLines := misc.ScanFile(filename)
			if debugLevel > 100 {
				fmt.Println("len(scanFileLines):\t", len(scanFileLines))
			}
		} else {
			readFileLines := misc.ReadFile(filename)
			if debugLevel > 100 {
				fmt.Println("len(readFileLines):\t", len(readFileLines))
			}
		}
	}

	// Clean up the test file
	errRemove := os.Remove(filename)
	if errRemove != nil {
		fmt.Println(errRemove)
	}
}

func BenchmarkScanFile100(b *testing.B) {
	benchmarkFileN(100, "scan", b)
}

func BenchmarkReadFile100(b *testing.B) {
	benchmarkFileN(100, "read", b)
}

func BenchmarkScanFile1000(b *testing.B) {
	benchmarkFileN(1000, "scan", b)
}

func BenchmarkReadFile1000(b *testing.B) {
	benchmarkFileN(1000, "read", b)
}

// TestCheckFilePermissions uses some common files to check permissions
func TestCheckFilePermissions(t *testing.T) {

	var tests = []struct {
		filename    string
		permissions string
		expected    bool
	}{
		{"/bin/bash", "0755", true}, // test 0
		{"/bin/ls", "0755", true},   // test 1
		//{"/etc/shadow", "0640", true},      // test 2 - can't test these in gitlab
		//{"/etc/sysctl.conf", "0644", true}, // test 3 - can't test these in gitlab
		//{"/etc/sudoers", "0440", true},     // test 4 - can't test these in gitlab
		{"/bin/bash", "0333", false}, // test 5 - negative
	}
	for i, test := range tests {

		if debugLevel > 10 {
			fmt.Println(i, "\ttest:\t", test)
		}

		if output := misc.CheckFilePermissions(test.filename, test.permissions); output != test.expected {
			t.Errorf("TestCheckFilePermissions Failed")
		}
	}

}
