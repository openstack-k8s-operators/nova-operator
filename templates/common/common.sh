#!/bin//bash
#
# Copyright 2018 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.

set -e

function get_ip_address_from_network {
  local network=$1
  # for now we expect that controller-0 is always there and has this hostname
  # format
  local controller_ip=$(getent -s hosts:files hosts controller-0.${network} | awk '{print $1}')
  local ip=$(ip route get ${controller_ip} | head -1 | awk '{print $5}')
  if [ -z "${ip}" ] ; then
    exit
  fi
  echo ${ip}
}

function get_ctrl_plane_ipaddress {
  # get the host from the http url
  local ctrl_plane_endpoint=$(echo $1 | awk -F[/:] '{print $4}')

  # get ip from host
  local ctrl_plane_ip=$(getent hosts ${ctrl_plane_endpoint} | awk '{print $1}')

  # get local ip
  local ip=$(ip route get ${ctrl_plane_ip} | head -1 | awk '{print $7}')
  if [ -z "${ip}" ] ; then
    exit
  fi
  echo ${ip}
}
