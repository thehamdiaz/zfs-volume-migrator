#!/bin/bash

zfs set mountpoint=legacy $POOLNAME/$DATASETNAME

# Sleep for testing
# sleep infinity