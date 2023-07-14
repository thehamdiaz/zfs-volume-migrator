#!/bin/bash

# Delete all volumesnapshots
kubectl delete volumesnapshot --all

# Delete all configmaps that start with "s"
kubectl get configmap --no-headers -o custom-columns=":metadata.name" |
  awk '/^s/{print $1}' |
  xargs -I {} kubectl delete configmap {}

# Delete all secrets that start with "s"
kubectl get secret --no-headers -o custom-columns=":metadata.name" |
  awk '/^s/{print $1}' |
  xargs -I {} kubectl delete secret {}

# Delete all jobs that start with "s"
kubectl get job --no-headers -o custom-columns=":metadata.name" |
  awk '/^s/{print $1}' |
  xargs -I {} kubectl delete job {}

# Delete all jobss that start with "legacy"
kubectl get job --no-headers -o custom-columns=":metadata.name" |
  awk '/^legacy-/{print $1}' |
  xargs -I {} kubectl delete job {}

# Delete all migrationrequests.api.k8s.zfs-volume-migrator.io objects
kubectl delete migrationrequest.api.k8s.zfs-volume-migrator.io --all

# Delete all restorerequests.api.k8s.zfs-volume-migrator.io objects
kubectl delete restorerequest.api.k8s.zfs-volume-migrator.io --all

# Delete all PVCs that start with "restore"
kubectl get pvc --no-headers -o custom-columns=":metadata.name" |
  awk '/^restored-/{print $1}' |
  xargs -I {} kubectl delete pvc {}

#apply the diffrent test objects
kubectl apply -f config/samples/api_v1_migrationrequest.yaml 
kubectl apply -f test-manifests/test-openebszfs/
kubectl apply -f test-manifests/random/my-fio.yaml
