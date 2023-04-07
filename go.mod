module github.com/Edgio/xtcp

go 1.16

replace github.com/Edgio/xtcp/pkg/misc => ./pkg/misc

replace github.com/Edgio/xtcp/pkg/blockfilter => ./pkg/blockfilter

replace github.com/Edgio/xtcp/pkg/cliflags => ./pkg/cliflags

replace github.com/Edgio/xtcp/pkg/xtcppb => ./pkg/xtcppb

replace github.com/Edgio/xtcp/pkg/poller => ./pkg/poller

replace github.com/Edgio/xtcp/pkg/netlinker => ./pkg/netlinker

replace github.com/Edgio/xtcp/pkg/inetdiager => ./pkg/inetdiager

replace github.com/Edgio/xtcp/pkg/pollerstater => ./pkg/pollerstater

replace github.com/Edgio/xtcp/pkg/inetdiagerstater => ./pkg/inetdiagerstater

replace github.com/Edgio/xtcp/pkg/netlinkerstater => ./pkg/netlinkerstater

require (
	github.com/go-cmd/cmd v1.3.0
	github.com/golang/protobuf v1.5.2
	github.com/nsqio/go-nsq v1.1.0 // indirect
	github.com/pkg/profile v1.6.0
	github.com/prometheus/client_golang v1.11.0
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	google.golang.org/protobuf v1.28.1
)
