#!/bin/bash

MNT="scratch/mount"
ROOT="scratch/root"

function FATAL() {
    echo "FATAL: $*" >&2
    exit 1
}

cleanup_files=""
function create_test_files() {
    touch $* || FATAL "could not create test files"
    cleanup_files="$cleanup_files $*"
}

cleanup_dirs=""
function create_test_dirs() {
    mkdir -p $* || FATAL "coult not create test directories"
    cleanup_dirs="$cleanup_dirs $*"
}

function setup() {
    create_test_dirs $ROOT/{empty,items} $ROOT $MNT scratch
    create_test_files $ROOT/file_in_root
    create_test_files $ROOT/items/{lowercase,UPPERCASE,MixedCase}
    create_test_files $ROOT/ambiguous_{file,FILE}

    make || exit 1
    ./icasefs -log_filename=icasefs.log $MNT $ROOT &

    # Hacky sleep to wait for the filesystem to come up.
    sleep 0.2
}

function teardown() {
    fusermount -u $MNT
    rm $cleanup_files
    rmdir $cleanup_dirs
}

failure=0
function FAIL() {
    echo "FAIL: $*" >&2
    failure=1
}

function ASSERT_EXISTS() {
    for f in $*; do
        [ -e "$f" ] || FAIL "Does not exist: $f"
    done
}

setup

# Files whose name exists as-is should exist as normal.
ASSERT_EXISTS $MNT/{,empty,items}
ASSERT_EXISTS $MNT/items/{lowercase,UPPERCASE,MixedCase}
ASSERT_EXISTS $MNT/{ambiguous_file,ambiguous_FILE}

# Files whose name case differences should exist
ASSERT_EXISTS $MNT/eMpTy
ASSERT_EXISTS $MNT/ItEms
ASSERT_EXISTS $MNT/items/{loWERCase,UPpercASE,MiXEDcase}

# Files whose parent directories' case differs should exist.
ASSERT_EXISTS $MNT/ItEms
ASSERT_EXISTS $MNT/iTEms/{loWERCase,UPpercASE,MiXEDcase}

# File that is ambiguous exists.
ASSERT_EXISTS $MNT/ambiguous_fILe

teardown

if [ "$failure" -eq "1" ]; then
    echo "Tests failed."
    exit 1
fi
