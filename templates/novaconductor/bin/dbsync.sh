#!/bin/bash

set -xe

if [ "${CELL_NAME}" = "cell0" ]; then
    nova-manage api_db sync
    CELL0DB=`crudini --get /etc/nova/nova.conf database connection`
    nova-manage cell_v2 map_cell0 --database_connection ${CELL0DB}
else
    # NOTE(gibi): Temporary hack. Register cell to the API DB. It is only
    # doable if the cell has access to the API DB. We should do this
    # in a Job in the Nova contorller instead.
    API_DB=`crudini --get /etc/nova/nova.conf api_database connection`
    if [ ! -z "API_DB" ]; then
        DB=`crudini --get /etc/nova/nova.conf database connection`
        MQ=`crudini --get /etc/nova/nova.conf DEFAULT transport_url`
        nova-manage cell_v2 create_cell --name "${CELL_NAME}" --database_connection "${DB}" --transport-url "${MQ}" --verbose
    fi
fi

nova-manage db sync --local_cell
