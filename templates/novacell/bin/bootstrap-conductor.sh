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

export Cell=${Cell:-"cell1"}
export DatabaseHost=${DatabaseHost:?"Please specify a DatabaseHost variable."}
export CellDatabase=${CellDatabase:-"nova_cell1"}
export TransportURL=${TransportURL:?"Please specify a TransportURL variable."}

# Bootstrap and exit if KOLLA_BOOTSTRAP variable is set. This catches all cases
# of the KOLLA_BOOTSTRAP variable being set, including empty.
if [[ "${!KOLLA_BOOTSTRAP[@]}" ]]; then
    # syncing cell conductor DBs on a per cell basis so that a
    # cell can be upgraded in isolation.
    nova-manage db sync --local_cell
    nova-manage db online_data_migrations
    exit 0
fi

if [[ "${!KOLLA_UPGRADE[@]}" ]]; then
    # syncing cell conductor DBs on a per cell basis so that a
    # cell can be upgraded in isolation.
    nova-manage db sync --local_cell
    exit 0
fi

if [[ "${!KOLLA_OSM[@]}" ]]; then
    nova-manage db online_data_migrations
    exit 0
fi
