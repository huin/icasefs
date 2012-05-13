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
    for d in $*; do
        mkdir $d || FATAL "could not create test directory $d"
        cleanup_dirs="$d $cleanup_dirs"
    done
}

function setup() {
    make || FATAL "Could not compile icasefs"

    mkdir scratch $MNT
    create_test_dirs $ROOT $ROOT/{empty,items}
    create_test_files $ROOT/file_in_root
    create_test_files $ROOT/items/{lowercase,UPPERCASE,MixedCase}
    create_test_files $ROOT/ambiguous_{file,FILE}

    ./icasefs -log_filename=icasefs.log $MNT $ROOT &

    # Hacky sleep to wait for the filesystem to come up.
    sleep 0.2
}

function teardown() {
    rm -f $cleanup_files
    rmdir $cleanup_dirs
    fusermount -u $MNT
    rmdir $MNT scratch
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

function ASSERT_CREATE_FILE() {
    touch $1 || FAIL "Could not create file: $1"
    cleanup_files="$cleanup_files $1"
}

function ASSERT_APPEND_FILE() {
    echo "appended" >> $1 || FAIL "Could not append to file: $1"
    cleanup_files="$cleanup_files $1"
}

function ASSERT_MKDIR() {
    mkdir $1 || FAIL "Could not mkdir: $1"
    cleanup_dirs="$1 $cleanup_dirs"
}

function ASSERT_CONTAINS() {
    grep -q $1 $2 || FAIL "File $2 does not contain $1"
}

function ASSERT_SYMLINK() {
    ln -s $1 $2 || FAIL "Could not create symlink: $2"
    cleanup_files="$cleanup_files $2"
}

function ASSERT_RENAME() {
    mv $1 $2 || FAIL "Count not rename $1 to $2"
    cleanup_files="$cleanup_files $2"
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

# Create file at root.
ASSERT_CREATE_FILE $MNT/created_file
ASSERT_EXISTS $MNT/created_file
# And append to it.
ASSERT_APPEND_FILE $MNT/created_file
ASSERT_CONTAINS appended $MNT/created_file

# Create file in directory with different case.
ASSERT_CREATE_FILE $MNT/ItEms/created_file

# Create symlink.
ASSERT_SYMLINK lowercase $MNT/ItEms/created_symlink
ASSERT_EXISTS $MNT/ItEms/created_symlink

# Create directory.
ASSERT_MKDIR $MNT/ItEms/created_directory

# File renaming.
ASSERT_RENAME $MNT/ItEms/created_file $MNT/renamed
ASSERT_EXISTS $MNT/renamed

ASSERT_CREATE_FILE $MNT/created_for_move
ASSERT_RENAME $MNT/created_for_move $MNT/ItEms/created_for_move
ASSERT_EXISTS $MNT/ItEms/created_for_move

teardown

if [ "$failure" -eq "1" ]; then
    echo "Tests failed."
    exit 1
fi
