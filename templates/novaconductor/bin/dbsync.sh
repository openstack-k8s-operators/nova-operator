#!/bin/bash

set -xe

CELL0=`crudini --get /etc/nova/nova.conf database connection`
nova-manage cell_v2 map_cell0 --database_connection $CELL0
nova-manage db sync