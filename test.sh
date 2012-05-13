#!/bin/bash
mkdir -p scratch/mount
(make && cp -a testdir scratch/testdir && ./icasefs scratch/mount scratch/testdir)
fusermount -u scratch/mount && rm -rf scratch/testdir
