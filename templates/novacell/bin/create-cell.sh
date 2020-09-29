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
set -x

export Cell=${Cell:-"cell1"}
export DatabaseHost=${DatabaseHost:?"Please specify a DatabaseHost variable."}
export CellDatabase=${CellDatabase:-"nova_cell1"}
export TransportURL=${TransportURL:?"Please specify a TransportURL variable."}

isCell=$(nova-manage cell_v2 list_cells | grep ${Cell})
if [ -z "${isCell}" ]; then
  echo "creating cell ${Cell}!"
  nova-manage cell_v2 create_cell --name ${Cell} \
    --database_connection "{scheme}://${CellDatabase}:{password}@${DatabaseHost}/${CellDatabase}" \
    --transport-url "${TransportURL}"
fi
