// Package cliflags is the struct for the cliFlags that gets passed to most of the xtcp go routines
//
// # This is kind of like how python passes the flags
//
// In the longer term, we might move all this to a config file of some type
package cliflags

import "time"

// CliFlags struct to make it easier to pass all the cli flags
type CliFlags struct {
	No4                       *bool
	No6                       *bool
	Timeout                   *int64
	PollingFrequency          *time.Duration
	PollingSafetyBuffer       *float64
	MaxLoops                  *int
	ShutdownWorkers           *bool
	Netlinkers4               *int
	Netlinkers6               *int
	Inetdiagers4              *int
	Inetdiagers6              *int
	Single                    *bool
	NlmsgSeq                  *int
	PacketSize                *int
	PacketSizeMply            *int
	NetlinkerChSize           *int
	SamplingModulus           *int
	InetdiagerReportModulus   *int
	InetdiagerStatsRatio      *float64
	GoMaxProcs                *int
	UDPSendDest               *string
	PromListen                *string
	PromPath                  *string
	PromPollerChSize          *int
	PromNetlinkerChSize       *int
	PromInetdiagerChSize      *int
	StatsdDst                 *string
	NoStatsd                  *bool
	HappyPollerReportModulus  *int
	HappyIstaterReportModulus *int
	NoDisabler                *bool
	DisablerFrequency         *time.Duration
	DisablerCommand           *string
	DisablerArgument1         *string
	DisablerArgument2         *string
	XTCPStaterFrequency       *time.Duration
	XTCPStaterSystemctlPath   *string
	XTCPStaterPsPath          *string
	NoLLDPer                  *bool
	LLDPOutputhPath           *string
	NoTrie                    *bool
	TrieCSV4                  *string
	TrieCSV6                  *string
	NoLoopback                *bool
	IPPath                    *string
	NSQ                       *string
}
