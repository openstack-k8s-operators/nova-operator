#!/bin/bash

assert_regex() {

    if [[ $1 =~ $2 ]]; then
        exit 0
    else
        printf '%s\n' "Regex check '$2' failed against oc output: $1";
        exit 1
    fi
}

"$@"
