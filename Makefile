#
# /xtcp/Makefile
#

# ldflags variables to update --version
# short commit hash
COMMIT :=$(shell /usr/bin/git describe --always)
DATE :=$(shell /bin/date -u +"%Y-%m-%d-%H:%M")

all: force

force: clean proto test xtcp

test:
	# can't do this yet, because not all modules have test yet
	# go test -v ./...
	chmod 755 ./pkg/disabler/testdata/return_one_after_X_runs.bash

	go test -v ./pkg/inetdiager/
	go test -v ./pkg/blockfilter/
	go test -v ./pkg/xtcpnl/
	go test -v ./pkg/disabler/
	go test -v ./pkg/xtcpstater/
	go test -v ./pkg/netlinker/
	go test -v ./pkg/misc/
	go test -v ./cmd/

	# das@das-dell5580:$ /usr/bin/find . -name '*_test.go'
	# ./pkg/inetdiager/inetdiager_test.go
	# ./pkg/xtcpnl/xtcpnl_test.go
	# ./pkg/disabler/disabler_test.go
	# ./pkg/xtcpstater/xtcpstater_test.go
	# ./pkg/netlinker/netlinker_test.go
	# ./pkg/misc/misc_test.go
	# ./cmd/xtcp_test.go

bench:
	#go test ./... -bench=.
	# reminder that "run" is the option to regex select which bench to run
	#go test -bench=. -run Trim

	go test -v ./pkg/inetdiager/ -bench=.
	go test -v ./pkg/blockfilter/ -bench=.
	go test -v ./pkg/xtcpnl/ -bench=.
	go test -v ./pkg/disabler/ -bench=.
	go test -v ./pkg/xtcpstater/ -bench=.
	go test -v ./pkg/netlinker/ -bench=.
	go test -v ./pkg/misc/ -bench=.
	go test -v ./cmd/ -bench=.

xtcp:
	rm -f ./bundle/bin/xtcp
	go build -ldflags "-X main.commit=${COMMIT} -X main.date=${DATE}" -o ./bundle/bin/xtcp ./cmd/xtcp.go

proto:
	protoc ./pkg/xtcppb/*.proto --go_out=.

fast:
	rm -f ./bundle/bin/xtcp
	go build -ldflags "-X main.commit=${COMMIT} -X main.date=${DATE}" -o ./bundle/bin/xtcp ./cmd/xtcp.go

clean:
	rm -f ./xtcp
	rm -f ./cmd/xtcp
	rm -f ./bundle/bin/xtcp 
	rm -f ./pkg/xtcppb/*.pb.*
	rm -f ./tools/xtcp_debug_server/xtcp_debug_server
	rm -f ./tools/xtcp_requester/xtcp_requester
	rm -f ./tools/xtcp_requester_ext/xtcp_requester_ext

clean_go_mod:
	/usr/bin/find . -type f -name 'go.mod' -print -delete
	/usr/bin/find . -type f -name 'go.sum' -print -delete
	go mod init
	# hack to use local modules before being commited
	cat go.mod.replace >> go.mod
	go mod tidy
	go mod verify

shellcheck:
	/usr/bin/shellcheck ./bundle/scripts/xtcp_wrapper.bash
	/usr/bin/shellcheck ./bundle/scripts/xtcp_update_systemd_and_restart.bash
	/usr/bin/shellcheck ./bundle/post-link-bundles
	/usr/bin/shellcheck ./bundle/smoke/xtcp_smoke.bash

update:
	go get github.com/go-cmd/cmd
	go get github.com/pkg/profile
	go get github.com/golang/protobuf
	go get github.com/prometheus/client_golang
	

#---------------------------------------
# Some testing tools
server:
	go build -o ./tools/xtcp_debug_server ./tools/xtcp_debug_server.go

requester:
	go build -o ./tools/xtcp_requester ./tools/xtcp_requester.go

requester_ext:
	go build -o ./tools/xtcp_requester_ext ./tools/xtcp_requester_ext.go

#---------------------------------------
# This runs xtcp at high frequencies.  Don't do this in prod!!
lots:
	./bundle/bin/xtcp -inetdiagerReportModulus 1 -samplingModulus 1 -frequency 5s

# go race detector.  seems to just run more slowly.
race:
	go run -race ./cmd/xtcp.go

# Lazy helper alias for scp
scp:
	scp ./bundle/bin/xtcp bast:

dodgy_sync:
	rsync -avz --exclude '.git' ./ ../xtcp_backup/

# tagging notes
# git tag
# git tag -a v1.0.5 -m "v1.0.5"
# git push origin --tags
