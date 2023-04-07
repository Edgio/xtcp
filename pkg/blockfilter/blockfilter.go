// Package blockfilter implements a filter that subsamples matching  traffic from the sampled data stream
// using data availible in a filter json

package blockfilter

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
)

const (
	debugLevel int = 10
)

// A container struct so we can swap out the inside list if we need to
type NetBlocks struct {
	BlockList []*net.IPNet
}

// Given an IP check if its talking to a host in the filter
func IsFilter(ip net.IP, filterBlocks *NetBlocks) bool {
	// If we didn't get set of blocks, just default out
	if filterBlocks == nil {
		return false
	}

	// Check to see if its in our set of filter blocks
	for _, currBlock := range filterBlocks.BlockList {
		if currBlock.Contains(ip) {
			return true
		}
	}
	return false
}

// Given a specific group, load the appropriate IP networks for the group from
// the filter json.
func LoadGroupNetworks(filterJson string, group string) *NetBlocks {
	var ipNets []*net.IPNet

	// Open our jsonFile
	jsonFile, err := os.Open(filterJson)
	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
	}
	// defer the closing of our jsonFile so that we can parse it later on
	defer jsonFile.Close()

	// Read the json
	byteValue, _ := ioutil.ReadAll(jsonFile)
	var result map[string]interface{}
	json.Unmarshal([]byte(byteValue), &result)

	// We need to assert the inside level to deal with the interface
	groupEntry, ok := result[group].(map[string]interface{})
	if !ok {
		if debugLevel > 100 {
			fmt.Println("Failed to assert json format at the group level!")
		}
		return nil
	}

	// Let's get the v4 networks from our current group
	v4Ranges := extractRanges(groupEntry["V4"])
	ipNets = append(ipNets, v4Ranges...)
	// Now let's get the v6
	v6Ranges := extractRanges(groupEntry["V6"])
	ipNets = append(ipNets, v6Ranges...)

	blocks := NetBlocks{ipNets}

	//Dump them out so we can see what it read
	if debugLevel > 100 {
		for _, network := range blocks.BlockList {
			fmt.Println(network)
		}
	}

	return &blocks
}

// Given a list of ranges from the Json, extract and convert them
func extractRanges(rawRange interface{}) []*net.IPNet {
	var ipNets []*net.IPNet

	// First, let's do a type assertion
	rangeList, ok := rawRange.([]interface{})
	if !ok {
		return nil
	}

	// Now let's just spin through all the types
	for _, rangeMap := range rangeList {
		// Type asert the range set
		netMap, ok := rangeMap.(map[string]interface{})
		if !ok {
			return nil
		}
		// Type assert each of the strings
		start, ok := netMap["StartIp"].(string)
		if !ok {
			return nil
		}
		end, ok := netMap["EndIp"].(string)
		if !ok {
			return nil
		}
		// Now we have the end points, go ahead and pass that to the build net
		// which will compute the appropriuate cidr
		newNet := buildIPNet(start, end)
		if newNet != nil {
			ipNets = append(ipNets, newNet)
		}
	}
	return ipNets
}

// Builld a network with the start and stop objects, filtering any junk
func buildIPNet(start string, end string) *net.IPNet {
	// If its RFC 1918 just blank
	ipStart := net.ParseIP(start)
	ipEnd := net.ParseIP(end)

	// The filter will ignore private blocks, to avoid wasting time checking on a case that
	// In the Prod case, these shouldn't happen in regular communications anyway.
	if ipStart.IsPrivate() {
		return nil
	}

	return RangeToCIDR(ipStart, ipEnd)
}

// Given two IP addresses, especially the start and end points of a range, find
// the smallest CIDR that contains both of them.
// This is adapted from this discussion: https://groups.google.com/g/golang-nuts/c/rJvVwk4jwjQ?pli=1
func RangeToCIDR(ip1 net.IP, ip2 net.IP) *net.IPNet {
	// Set the max length according to the IP version.
	// THis trick uses the fact that to4 only works for v4
	var maxLen int
	if ip1.To4() != nil {
		maxLen = 32
	} else {
		maxLen = 128
	}

	// Just iterate with larger and larger subnets until we get one that includes both
	for l := maxLen; l >= 0; l-- {
		// Build a net object so we can just use contains
		mask := net.CIDRMask(l, maxLen)
		na := ip1.Mask(mask)
		n := net.IPNet{IP: na, Mask: mask}
		// If we hit a size that has ip2, we are done return it and leave
		if n.Contains(ip2) {
			return &n
		}
	}
	// I dont think this should exactly be possible, but you never know!
	return nil
}
