#
# /cmd/Makefile
#

# ldflags variables to update --version
# short commit hash
COMMIT :=$(shell /usr/bin/git describe --always)
DATE :=$(shell /bin/date -u +"%Y-%m-%d-%H:%M")

all: clean xtcp

test:
	go test

clean:
	/bin/rm -rf ./xtcp

xtcp:
	go build -ldflags "-X main.commit=${COMMIT} -X main.date=${DATE}" -o ./xtcp ./xtcp.go
