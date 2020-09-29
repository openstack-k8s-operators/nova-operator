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

export DatabasePassword=${DatabasePassword:?"Please specify a DatabasePassword variable."}
export DatabaseHost=${DatabaseHost:?"Please specify a DatabaseHost variable."}
export CellDatabase=${CellDatabase:-"nova_cell0"}
export DatabaseConnection="mysql+pymysql://$CellDatabase:$DatabasePassword@$DatabaseHost/$CellDatabase"

# Bootstrap and exit if KOLLA_BOOTSTRAP variable is set. This catches all cases
# of the KOLLA_BOOTSTRAP variable being set, including empty.
if [[ "${!KOLLA_BOOTSTRAP[@]}" ]]; then
    nova-manage api_db sync
    nova-manage cell_v2 map_cell0 --database_connection "${DatabaseConnection}"
    nova-manage db sync
    nova-manage db online_data_migrations
    exit 0
fi

if [[ "${!KOLLA_UPGRADE[@]}" ]]; then
    nova-manage api_db sync
    nova-manage db sync
    exit 0
fi

if [[ "${!KOLLA_OSM[@]}" ]]; then
    nova-manage db online_data_migrations
    exit 0
fi


