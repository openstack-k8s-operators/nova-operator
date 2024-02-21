#!/bin/bash
# Copyright 2024.
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

set -x
export ARCHIVE_AGE=${ARCHIVE_AGE:?"Please specify ARCHIVE_AGE variable."}
export PURGE_AGE=${PURGE_AGE:?"Please specify PURGE_AGE variable."}
archive_before=$(date --date="${ARCHIVE_AGE} day ago" +%Y-%m-%d)
purge_before=$(date --date="${PURGE_AGE} day ago" +%Y-%m-%d)

nova-manage db archive_deleted_rows --verbose --until-complete --task-log --before "${archive_before}"
ret=$?
# 0 means no error and nothing is archived
# 1 means no error and someting is archived
if [ $ret -gt "1" ]; then
    exit $ret
fi

nova-manage db purge --verbose --before "${purge_before}"
ret=$?
# 0 means no error and something is deleted
# 3 means no error and nothing is deleted
if [[ $ret -eq 1 || $ret -eq 2 || $ret -gt 3 ]]; then
    exit $ret
fi

exit 0
