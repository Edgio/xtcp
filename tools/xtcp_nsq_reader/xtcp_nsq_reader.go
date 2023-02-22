package main

// Utility to read xtcp messages from NSQ

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Edgio/xtcp/pkg/xtcppb"
	"github.com/nsqio/go-nsq"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func messageHandler(message *nsq.Message) error {
	//fmt.Println(string(message.Body))
	XtcpRecord := &xtcppb.XtcpRecord{}
	err := proto.Unmarshal(message.Body, XtcpRecord)
	if err != nil {
		fmt.Println("proto.Unmarshal(message.Body, XtcpRecord) error:", err)
	}
	XtcpRecordJSON := protojson.Format(XtcpRecord)
	if err != nil {
		fmt.Println("protojson.Format(XtcpRecord) error: ", err)
	}
	fmt.Println(XtcpRecordJSON)
	return nil
}

func main() {
	config := nsq.NewConfig()
	consumer, err := nsq.NewConsumer("xtcp", "channel-1", config)
	if err != nil {
		log.Fatal(err)
	}

	consumer.AddHandler(nsq.HandlerFunc(messageHandler))
	err = consumer.ConnectToNSQD("localhost:4150")
	if err != nil {
		log.Fatal(err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

}
