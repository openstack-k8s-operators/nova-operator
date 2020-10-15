#!/bin/bash
#
# Copyright 2020 Red Hat Inc.
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
set -ex

# This script generates the nova.conf/logging.conf file and
# copies the result to the ephemeral /var/lib/config-data/merged volume.
#
# Secrets are obtained from ENV variables.
export PodIP=${PodIP:?"Please specify a PodIP variable."}
export NovaKeystoneAuthPassword=${NovaKeystoneAuthPassword:?"Please specify a NovaKeystoneAuthPassword variable."}
export PlacementKeystoneAuthPassword=${PlacementKeystoneAuthPassword:?"Please specify a PlacementKeystoneAuthPassword variable."}
export NeutronKeystoneAuthPassword=${NeutronKeystoneAuthPassword:?"Please specify a NeutronKeystoneAuthPassword variable."}
export TransportURL=${TransportURL:?"Please specify a TransportURL variable."}

# expect that the common.sh is in the same dir as the calling script
SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
. ${SCRIPTPATH}/common.sh --source-only

# Copy default service config from container image as base
cp -a /etc/nova/nova.conf /var/lib/config-data/merged/nova.conf

# Merge all templates from config-data and config-data-custom CMs
for dir in /var/lib/config-data/default /var/lib/config-data/custom
do
  merge_config_dir ${dir}
done

# Create local instances directory inside /var/lib/nova which gets bind
# mounted into the container
mkdir -p /var/lib/nova/instances
chown nova:nova /var/lib/nova/instances

# configure host specific mandatory settings
crudini --set /var/lib/config-data/merged/nova.conf DEFAULT my_ip ${PodIP}
crudini --set /var/lib/config-data/merged/nova.conf libvirt live_migration_inbound_addr ${PodIP}
crudini --set /var/lib/config-data/merged/nova.conf vnc server_listen ${PodIP}
# TODO: vnc
crudini --set /var/lib/config-data/merged/nova.conf vnc server_proxyclient_address ${PodIP}

# set secrets
crudini --set /var/lib/config-data/merged/nova.conf DEFAULT transport_url $TransportURL
crudini --set /var/lib/config-data/merged/nova.conf keystone_authtoken password $NovaKeystoneAuthPassword
crudini --set /var/lib/config-data/merged/nova.conf neutron password $NeutronKeystoneAuthPassword
crudini --set /var/lib/config-data/merged/nova.conf placement password $PlacementKeystoneAuthPassword