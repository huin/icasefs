#!/bin/bash

MNT="scratch/mount"
ROOT="scratch/root"

function setup() {
    mkdir -p "$MNT" || exit 1

    mkdir -p "$ROOT/"{empty,items}
    touch "$ROOT/items/emptyfile"
    echo file_in_root > "$ROOT/file_in_root"
    echo lowercase > "$ROOT/items/lowercase"
    echo UPPERCASE > "$ROOT/items/UPPERCASE"
    echo MixedCase > "$ROOT/items/MixedCase"

    make || exit 1
    ./icasefs -log_filename=icasefs.log "$MNT" "$ROOT" &

    # Hacky sleep to wait for the filesystem to come up.
    sleep 1
}

function teardown() {
    fusermount -u "$MNT"
    rm "$ROOT"/file_in_root \
        "$ROOT"/items/{emptyfile,lowercase,UPPERCASE,MixedCase}
    rmdir "$ROOT"/{empty,items} "$ROOT" "$MNT" scratch
}

failure=0
function FAIL() {
    echo $* >&2
    failure=1
}

function ASSERT_EXISTS() {
    [ -e "$1" ] || FAIL "Does not exist: $1"
}

setup

# Files whose name exists as-is should exist as normal.
ASSERT_EXISTS "$MNT/"
ASSERT_EXISTS "$MNT/empty"
ASSERT_EXISTS "$MNT/items"
ASSERT_EXISTS "$MNT/items/lowercase"
ASSERT_EXISTS "$MNT/items/UPPERCASE"
ASSERT_EXISTS "$MNT/items/MixedCase"

# Files whose name case differences should exist
ASSERT_EXISTS "$MNT/eMpTy"
ASSERT_EXISTS "$MNT/ItEms"
ASSERT_EXISTS "$MNT/items/loWERCase"
ASSERT_EXISTS "$MNT/items/UPpercASE"
ASSERT_EXISTS "$MNT/items/MiXEDcase"

# Files whose parent directories' case differs should exist.
ASSERT_EXISTS "$MNT/ItEms"
ASSERT_EXISTS "$MNT/iTEms/loWERCase"
ASSERT_EXISTS "$MNT/iTEms/UPpercASE"
ASSERT_EXISTS "$MNT/iTEms/MiXEDcase"

teardown

if [ "$failure" -eq "1" ]; then
    echo "Tests failed."
    exit 1
fi
