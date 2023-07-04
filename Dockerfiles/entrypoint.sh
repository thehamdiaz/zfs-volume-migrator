#!/bin/bash

# Copy ssh keys from the secret
cp /etc/ssh-key/id_rsa /root/.ssh/
cp /etc/ssh-key/id_rsa.pub /root/.ssh/

# Fix permistions
chmod 400 /root/.ssh/id_rsa

# Check if previous snapshot is "None"
if [[ $PREVIOUS == "None" ]]; then
    # Send the snapshot to the remote node
    zfs send $SNAPSHOT | ssh -o StrictHostKeyChecking=no $USER@$REMOTE_HOST zfs receive -u $POOL/$DATASET
else
    zfs send -i $PREVIOUS $SNAPSHOT | ssh -o StrictHostKeyChecking=no $USER@$REMOTE_HOST zfs receive -u $POOL/$DATASET
fi

# Sleep for testing
# sleep infinity