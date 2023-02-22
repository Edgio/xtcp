// Package misc are some small helper functions used throughout the xtcp code
//
// It is perhaps poor form to name a module "misc", could be renamed to "utils"
package misc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
)

const (
	debugLevel int = 11
)

var (
	// KernelEnumToString maps kernel enum to human string for the IP protocol
	KernelEnumToString = map[uint8]string{
		uint8(2):  "v4",
		uint8(10): "v6",
	}
)

// DieIfNotLinux as the name suggests kills this program if we aren't running on linux
// We only support Linux
// Although I think Darwin has netlink also
// TODO - test Darwin
func DieIfNotLinux() {
	if runtime.GOOS != "linux" {
		log.Fatal("This code is only designed for linux.")
	}
}

// GetHostname is a little gethostname helper with fatal error check
// arguably this function shouldn't exist
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("os.Hostname() error:%s", err)
	}
	if debugLevel > 100 {
		fmt.Println("Hostname:", hostname)
	}
	return hostname
}

// MaxLoopsOrForEver returns true if maxloops == 0, or pollingLoops < maxloops
// This function just allows us to embed if logic into the main pollingLoops for statement
func MaxLoopsOrForEver(pollingLoops int, maxLoops int) bool {
	if maxLoops != 0 {
		if pollingLoops > maxLoops {
			return false
		}
	}
	return true
}

// ScanFile reads a file and returns the lines as a slice
// Pleae note that the sc.Scan() strips the "\n", so this is being added back
func ScanFile(file string) []string {
	var lines []string

	f, err := os.OpenFile(file, os.O_RDONLY, os.ModePerm)
	if err != nil {
		log.Fatalf("XTCPStater scanFile open file error: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line+string('\n'))
	}
	if err := sc.Err(); err != nil {
		log.Fatalf("XTCPStater scanFile scan file error: %v", err)
	}
	return lines
}

// ReadFile reads a file and returns the lines as a slice
// Recommend using ScanFile because bufio.NewScanner is apparently faster
// Only including ReadFile to allow testing of ScanFile
// Compared to sc.Scan() above, rd.ReadString('\n') does NOT strip the '\n'
func ReadFile(file string) []string {
	var lines []string

	f, err := os.OpenFile(file, os.O_RDONLY, os.ModePerm)
	if err != nil {
		log.Fatalf("open file error: %v", err)
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			log.Fatalf("read file line error: %v", err)
		}
		lines = append(lines, line)
	}
	return lines
}

// CheckFilePermissions checks the permission bits on a filename 0755
// e.g. pass filename and the permissions you want to check
// This is a crude string comparisions, and does NOT look at who
// is running this code, or the ownership of the file in question
func CheckFilePermissions(filename string, permissions string) bool {
	// https://golang.org/pkg/os/#Stat
	// https://golang.org/pkg/os/#FileInfo
	fileStat, err := os.Stat(filename)
	if err != nil {
		log.Fatal("checkFilePermissions os.Stat error:", filename, err)
	}
	filePerms := fmt.Sprintf("%#o", fileStat.Mode().Perm())
	if filePerms != permissions {
		if debugLevel > 10 {
			fmt.Println("filePerms:", filePerms, "\t do NOT match:", permissions)
		}
		return false
	}
	return true
}

// byteToMegabyte is a simple byte to megabyte helper function
func byteToMegabyte(b uint64) uint64 {
	return b / 1024 / 1024
}

// PrintMemUsage uses the go runtime libary to print out current memory usage
func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", byteToMegabyte(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", byteToMegabyte(m.TotalAlloc))
	fmt.Printf("\tSys = %v MiB", byteToMegabyte(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}
