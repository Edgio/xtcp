#!/bin/bash

apt update 
readarray -t package_list < <(sed 's/#.*$//;/^[[:space:]]*$/d' required-debs.txt)
DEBIAN_FRONTEND=noninteractive apt install -y -- "${package_list[@]}"

# Setup Golang 
wget https://go.dev/dl/go1.19.5.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.19.5.linux-amd64.tar.gz
rm  go1.19.5.linux-amd64.tar.gz
touch /home/vagrant/.bash_profile
echo "export PATH=$PATH:/usr/local/go/bin" >> /home/vagrant/.bash_profile
source /home/vagrant/.bash_profile

export GOPATH=$(go env GOPATH)
export PATH=$GOPATH/bin:$PATH
echo "export PATH=$PATH:/usr/local/go/bin" >> /home/vagrant/.bash_profile
source /home/vagrant/.bash_profile
go version

# Install protoc 3.17
curl --location --remote-name https://github.com/protocolbuffers/protobuf/releases/download/v3.17.3/protoc-3.17.3-linux-x86_64.zip
unzip protoc-3.17.3-linux-x86_64.zip bin/protoc
rm protoc-3.17.3-linux-x86_64.zip
cp ./bin/protoc /usr/bin/
rm -rf ./bin
chmod 755 /usr/bin/protoc
chown root:root /usr/bin/protoc

go get google.golang.org/protobuf/proto
go install google.golang.org/protobuf/cmd/protoc-gen-go
which protoc
which protoc-gen-go

# Install NSQ
wget https://s3.amazonaws.com/bitly-downloads/nsq/nsq-1.2.1.linux-amd64.go1.16.6.tar.gz
tar xzf nsq-1.2.1.linux-amd64.go1.16.6.tar.gz
cp nsq-1.2.1.linux-amd64.go1.16.6/bin/* /usr/bin/
rm -rf nsq-1.2.1.linux-amd64.go1.16.6 nsq-1.2.1.linux-amd64.go1.16.6.tar.gz
nsqd &
nsqlookupd &

# Make xtcp
make clean
make proto
make xtcp

# Setup Environment Variables
echo "export XTCP_DISABLED=0" >> /home/vagrant/.bash_profile
echo "export XTCP_FREQUENCY="2s"" >> /home/vagrant/.bash_profile
echo "export XTCP_FREQUENCY_LENGTH=2" >> /home/vagrant/.bash_profile
echo "export XTCP_SAMPLING_MODULUS="default"" >> /home/vagrant/.bash_profile
echo "export XTCP_REPORT_MODULUS=1" >> /home/vagrant/.bash_profile
echo "export XTCP_NSQ="localhost:4150"" >> /home/vagrant/.bash_profile
source /home/vagrant/.bash_profile
printenv | grep XTCP
