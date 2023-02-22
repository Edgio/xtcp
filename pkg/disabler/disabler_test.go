// Package disabler_test is a basic test of the disabler go routine

// Please note the completion time is quite long (around >6 seconds) and is expected

package disabler_test

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/disabler"
)

const (
	debugLevel int = 11
)

// TestDisabler does some basic checks for the disabler.Disabler function
// This leverages a bash script (TODO rewrite in go) and runs a little slowly
func TestDisabler(t *testing.T) {
	var tests = []struct {
		frequency time.Duration
		command   string
		argument1 string
		argument2 string
		loops     int
		expected  bool
	}{
		{500 * time.Millisecond, "./testdata/return_one_after_X_runs.bash", "--default=0", "2", int(3), true},
		{500 * time.Millisecond, "./testdata/return_one_after_X_runs.bash", "--default=0", "3", int(4), true},
		{500 * time.Millisecond, "./testdata/return_one_after_X_runs.bash", "--default=0", "4", int(5), true},
		{500 * time.Millisecond, "/bin/false", "--default=0", "blah", int(10), true},
	}

	if debugLevel > 10 {
		fmt.Println("TestDisabler should take just over 5> seconds <10 (expected)")
	}

	for _, test := range tests {

		var cliFlags cliflags.CliFlags
		cliFlags.DisablerFrequency = &test.frequency
		cliFlags.MaxLoops = &test.loops
		cliFlags.DisablerCommand = &test.command
		cliFlags.DisablerArgument1 = &test.argument1
		cliFlags.DisablerArgument2 = &test.argument2

		err := os.Remove("./testdata/tmp_counter") // remove a single file
		if err != nil {
			fmt.Println(err)
		}
		if debugLevel > 10 {
			fmt.Println("test:\t", test)
		}

		// note the buffered channel, which needs to be at least a large as the number of tests, so we don't need to worry about draining
		disablerCheckComplete := make(chan struct{}, 10)
		if output := disabler.Disabler(cliFlags, disablerCheckComplete, true); output != test.expected {
			// https://golang.org/pkg/testing/#B.Error
			t.Errorf("Test Failed: frequency %s, command %s, argument1 %s, argument2 %s, loops %s, expected %s, output %s",
				fmt.Sprint(test.frequency),
				test.command,
				test.argument1,
				test.argument2,
				fmt.Sprint(test.loops),
				strconv.FormatBool(test.expected),
				strconv.FormatBool(output))
		} else {
			if debugLevel > 100 {
				fmt.Println("test:\tSuccess")
			}
		}

		if debugLevel > 100 {
			fmt.Println("Disable check complete")
		}
	}
}

// The following were some quick benchmarks to help decide if we should TrimSpace or not.  = We should TrimSpace.

var stringToAvoidOptimiztion string

func BenchmarkOneByteNoTrim(b *testing.B) {

	myBytes := []byte{'1'}

	if debugLevel > 100 {
		fmt.Println("myBytes:", myBytes)
		fmt.Println("string(myBytes):", string(myBytes))
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		if string(myBytes) == "1" {
			stringToAvoidOptimiztion = string(myBytes)
		}
	}

	stringToAvoidOptimiztion = string(myBytes)
}

func BenchmarkTwoBytesNoTrim(b *testing.B) {

	myBytes := []byte{'1', '\n'}

	if debugLevel > 100 {
		fmt.Println("myBytes:", myBytes)
		fmt.Println("string(myBytes):", string(myBytes))
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		if string(myBytes) == "1\n" {
			stringToAvoidOptimiztion = string(myBytes)
		}
	}

	stringToAvoidOptimiztion = string(myBytes)
}

func BenchmarkTwoBytesTrim(b *testing.B) {

	myBytes := []byte{'1', '\n'}

	if debugLevel > 100 {
		fmt.Println("myBytes:", myBytes)
		fmt.Println("string(myBytes):", string(myBytes))
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		if string(bytes.TrimSpace(myBytes)) == "1" {
			stringToAvoidOptimiztion = string(bytes.TrimSpace(myBytes))
		}
	}

	stringToAvoidOptimiztion = string(myBytes)
}
