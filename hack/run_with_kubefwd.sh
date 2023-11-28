#!/bin/bash
set -ex

pushd bin
wget https://github.com/txn2/kubefwd/releases/latest/download/kubefwd_Linux_x86_64.tar.gz
tar xvf kubefwd_Linux_x86_64.tar.gz
rm kubefwd_Linux_x86_64.tar.gz
popd


function cleanup()
{
    sudo pkill kubefwd
}

trap cleanup SIGINT SIGTERM EXIT

sudo -E ./bin/kubefwd services -n openstack &

# wait for the port forwarding to start up
sleep 5

go run ./main.go -metrics-bind-address ":$(METRICS_PORT)" -health-probe-bind-address ":$(HEALTH_PORT)"
