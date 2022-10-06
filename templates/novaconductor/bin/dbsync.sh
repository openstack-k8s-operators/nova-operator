#!/bin/bash

set -xe

if [ "${CELL_NAME}" = "cell0" ]
then
    nova-manage api_db sync
    CELL0DB=`crudini --get /etc/nova/nova.conf database connection`
    nova-manage cell_v2 map_cell0 --database_connection ${CELL0DB}
fi

nova-manage db sync
