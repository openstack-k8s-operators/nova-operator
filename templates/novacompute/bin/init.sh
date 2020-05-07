#!/bin//bash
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

# expect that the common.sh is in the same dir as the calling script
SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
. ${SCRIPTPATH}/common.sh --source-only

if [ -z "${CONFIG_VOLUME}" ] ; then
  echo "No config volume specified!"
  exit 1
fi

if [ -z "${TEMPLATES_VOLUME}" ] ; then
  echo "No templates volume specified!"
  exit 1
fi

# Create local instances directory inside /var/lib/nova which gets bind
# mounted into the container
mkdir -p /var/lib/nova/instances
chown nova:nova /var/lib/nova/instances

# copy configs to the CONFIG_VOLUME config volume
mkdir -p ${CONFIG_VOLUME}/etc/nova
cp -L ${TEMPLATES_VOLUME}/* ${CONFIG_VOLUME}/etc/nova/

# configure host specific mandatory settings
LOCAL_IP=$(get_ip_address_from_network "internalapi")
crudini --set ${CONFIG_VOLUME}/etc/nova/nova.conf DEFAULT my_ip ${LOCAL_IP}
crudini --set ${CONFIG_VOLUME}/etc/nova/nova.conf libvirt live_migration_inbound_addr ${LOCAL_IP}
crudini --set ${CONFIG_VOLUME}/etc/nova/nova.conf vnc server_listen ${LOCAL_IP}
crudini --set ${CONFIG_VOLUME}/etc/nova/nova.conf vnc server_proxyclient_address ${LOCAL_IP}
