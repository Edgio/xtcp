package main

// This is a really quick little UDP server that will recieve
// the UDP binary protobuf and will print to json
// This is just to prove it works

import (
	"flag"
	"fmt"
	"net"
	"syscall"

	"github.com/Edgio/xtcp/pkg/xtcppb" // xtcp protobuf
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {

	udpListen := flag.String("udpListen", "127.0.0.1:13000", "UDP socket to listen. Deafult = 127.0.0.1:13000")
	flag.Parse()

	serverAddr, err := net.ResolveUDPAddr("udp", *udpListen)
	connection, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer connection.Close()
	fmt.Println("listening:", *udpListen)

	buffer := make([]byte, syscall.Getpagesize()/2)
	for {
		n, addr, err := connection.ReadFromUDP(buffer)
		fmt.Println("addr:", addr)
		fmt.Println("n:", n)
		mybuffer := buffer[:n]
		XtcpRecord := &xtcppb.XtcpRecord{}
		err = proto.Unmarshal(mybuffer, XtcpRecord)
		if err != nil {
			fmt.Println("proto.Unmarshal(mybuffer, XtcpRecord) error:", err)
		}
		XtcpRecordJSON := protojson.Format(XtcpRecord)
		if err != nil {
			fmt.Println("protojson.Format(XtcpRecord) error: ", err)
		}
		fmt.Println(XtcpRecordJSON)
	}
}
