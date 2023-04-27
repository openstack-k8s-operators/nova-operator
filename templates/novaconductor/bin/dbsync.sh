#!/bin/bash

set -xe
CONF_FILE="/etc/nova/nova.conf.d/01-nova.conf"

if [ "${CELL_NAME}" = "cell0" ]; then
    nova-manage api_db sync
    CELL0DB=`crudini --get ${CONF_FILE} database connection`
    nova-manage cell_v2 map_cell0 --database_connection ${CELL0DB}
else
    # NOTE(gibi): Temporary hack. Register cell to the API DB. It is only
    # doable if the cell has access to the API DB. We should do this
    # in a Job in the Nova contorller instead.
    API_DB=`crudini --get ${CONF_FILE} api_database connection`
    if [ ! -z "API_DB" ]; then
        DB=`crudini --get ${CONF_FILE} database connection`
        MQ=`crudini --get ${CONF_FILE} DEFAULT transport_url`
        nova-manage cell_v2 create_cell --name "${CELL_NAME}" --database_connection "${DB}" --transport-url "${MQ}" --verbose > /tmp/create-cell.out || grep "The specified transport_url and/or database_connection combination already exists" /tmp/create-cell.out
    fi
fi

nova-manage db sync --local_cell
