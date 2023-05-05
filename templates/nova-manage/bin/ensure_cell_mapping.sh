#!/bin/bash
# Copyright 2023.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -xe

export CELL_NAME=${CELL_NAME:?"Please specify a CELL_NAME variable."}

# NOTE(gibi): nova-manage should be enhanced upstream to get rid of this
# uglyness
cell_uuid=$(nova-manage cell_v2 list_cells | tr ' ' '|' | tr --squeeze-repeats '|' | grep "$CELL_NAME" | cut -d '|' -f 3)

if [ -z "${cell_uuid}" ]; then
    if [ "${CELL_NAME}" = "cell0" ]; then
        nova-manage cell_v2 map_cell0
        # NOTE(gibi): cell_v2 map_cell0 command above blindly appended _cell0 to the end
        # of the DB URL found in the configuration and ended up with nova_cell0_cell0 which
        # is invalid. This is an nova upstream bug to fix. But we can workaround it as
        # update_cell does have the corret behavior when collecting the cell mapping information
        # from the nova.conf
        nova-manage cell_v2 update_cell --cell_uuid 00000000-0000-0000-0000-000000000000
    else
        nova-manage cell_v2 create_cell --name "${CELL_NAME}" --verbose
    fi

else
    nova-manage cell_v2 update_cell --cell_uuid "${cell_uuid}"
fi
