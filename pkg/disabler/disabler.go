// Package disabler contains a go routine that polls system variable at a regular interval
// and will kill xtcp if the stdout response is "1\n"
//
// This go routine can be disabled via cli flag.
//
// Please note that given xtcp should NOT be running as root, so this code does NOT carefully check the command arguments passed
// If running as root, this could be dangerous, but root is already dangerous enough, so we not making things worse
//
// The channel is required to ensure that on startup xtcp blocks waiting for the first iteration to complete, otherwise polling can start
// before we've finished checking if we're disabled (eg spawning the new process is slower than starting the other go routines)
package disabler

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/Edgio/xtcp/pkg/cliflags"
	"github.com/Edgio/xtcp/pkg/misc"
)

const (
	debugLevel int = 11
)

// Disabler polls a command, and if the stdout response is "1\n", the calls os.Exit(0) - a clean exit
// It goes return bool, for when using for testing.  Disabler == true means it would os.Exit(0) if not testing
func Disabler(cliFlags cliflags.CliFlags, done chan<- struct{}, testing bool) bool {

	ticker := time.NewTicker(*cliFlags.DisablerFrequency)

	if debugLevel > 100 {
		fmt.Println("Disabler start")
	}

	//if !misc.CheckFilePermissions(*cliFlags.DisablerCommand, "0755") {
	//	if debugLevel > 10 {
	//		fmt.Println("DisablerCommand does not have 0755 permissions:", *cliFlags.DisablerCommand) // extra log cos the fatal log messages doesn't make it to the systemd journal for some reason
	//		log.Fatal("DisablerCommand does not have 0755 permissions:", *cliFlags.DisablerCommand)
	//	}
	//}

	for pollingLoops := 0; misc.MaxLoopsOrForEver(pollingLoops, *cliFlags.MaxLoops); pollingLoops++ {

		response, err := exec.Command(*cliFlags.DisablerCommand, *cliFlags.DisablerArgument1).Output()
		fmt.Println("*response")
		if err != nil {
			if testing {
				if debugLevel > 10 {
					fmt.Println("*cliFlags.DisablerCommand failed, and this would normally be bad, but this is a test, so it's ok.")
				}
				return true
			}
			if debugLevel > 10 {
				log.Fatal("Disabler exec.Command err:", err)
			}
		}
		if debugLevel > 100 {
			fmt.Print("Disabler:\tstring(response):", string(response))
		}
		if string(bytes.TrimSpace(response)) == "1" {
			if testing {
				if debugLevel > 10 {
					fmt.Println("xtcp is disabled, but this is a test, so it's ok.")
				}
				return true
			}
			if debugLevel > 10 {
				fmt.Println("xtcp is disabled, so exiting cleanly (exit(0)). err:", err)
			}
			os.Exit(0) // Exit for real!
		}

		if pollingLoops == 0 && !testing {
			if debugLevel > 10 {
				fmt.Println("xtcp is enabled")
			}
			done <- struct{}{}
			if debugLevel > 100 {
				fmt.Println("Disabler notified main.")
			}
		}

		<-ticker.C
		if debugLevel > 100 {
			fmt.Println("Disabler tick")
		}

		// // select not really required, but we could add a HTTP hook or signal handler here
		// select {
		// case <-ticker.C:
		// 	if debugLevel > 100 {
		// 		fmt.Println("Disabler tick")
		// 	}
		// 	break
		// 	// default:
		// 	// 	//nothing
		// }
	}
	return true
}
