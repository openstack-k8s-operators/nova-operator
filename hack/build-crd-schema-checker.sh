#!/bin/bash
set -euxo pipefail

if [ -f "$INSTALL_DIR/crd-schema-checker" ]; then
    exit 0
fi
if [ -f ~/crd-schema-checker ]; then
    cp ~/crd-schema-checker "$INSTALL_DIR/"
    exit
fi

mkdir -p "$INSTALL_DIR/git-tmp"
git clone https://github.com/openshift/crd-schema-checker.git \
    -b "$CRD_SCHEMA_CHECKER_VERSION" "$INSTALL_DIR/git-tmp" || true
pushd "$INSTALL_DIR/git-tmp"
grep -q 'SHELL=/bin/bash' Makefile || sed -i '1 i\SHELL=/bin/bash' Makefile
GOWORK=off make
cp crd-schema-checker ~/
cp crd-schema-checker "$INSTALL_DIR/"
popd
rm -rf "$INSTALL_DIR/git-tmp"
