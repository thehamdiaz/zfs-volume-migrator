# Steps to build the project environment

You need to have 3 nodes (virtual machines) with 4GB of RAM and 2vCPU and 40GB of storage each.

## Setting up the nodes.
You need to have 3 nodes (virtual machines) with 4GB of RAM and 2vCPU and 40GB of storage each.
You need to:
1. Install Ubuntu 20.04 OS on each node.
2. configure network settings in the virtual machines. Ensure that all the machines are connected to each other and can communicate with each other.
3. Install and enable ssh on each of the nodes.
```
$ sudo apt-get install openssh-server -y
```
4. Set up meaningful host names for each node (controlplane, worker1, worker2)
```
$ sudo hostnamectl set-hostname <hostname>
```
5. Disable swap on each node and also comment out swap entry in `/etc/fstabfile`
```
$ swapoff -a
```
Install necessary Packages and add the kubernetes repo on each node’s system
```
sudo apt-get update && sudo apt-get install -y apt-transport-https curl
sudo curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF | sudo tee /etc/apt/sources.list.d/kubernetes.list deb https://apt.kubernetes.io/ kubernetes-xenial main EOF
```

## Setting up the cluster

### Getting ready:

1. Install kubeadm, kubectl and kubelet on each node:
``` 
$ sudo apt-get update
$ sudo apt-get install -y kubelet kubeadm kubectl
```
2. Install Container Runtime on each node: (execute this as root)
```
$ VERSION=1.27
$ OS=xUbuntu_20.04
$ echo "deb https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
$ echo "deb http://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/ /" > /etc/apt/sources.list.d/devel:kubic:libcontainers:stable:cri-o:$VERSION.list

$ curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable:/cri-o:/$VERSION/$OS/Release.key | apt-key add -
$ curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/$OS/Release.key | apt-key add -

$ apt-get update
$ apt-get install cri-o cri-o-runc
```
3. Install the control plane:
In the master node, execute `kubeadm init` command to deploy control plane components:
```
$ kubeadm init --pod-network-cidr=192.168.2.0/16
```
When the above command execution is successful, it will yield a command to be executed on all the worker nodes to configure them with the master.
4. Join the worker nodes:
After configuring the master node successfully, configure each worker node by executing the join command displayed in master node:
```
kubeadm join x.x.x.x:6443 --token <token> --discovery-token-ca-cert-hash <hash>
```

4. Accessing Cluster:
from the master node you can communicate with the cluster components using the `kubectl` interface. In order to communicate, you need the kubernetes cluster config file to be placed in the home directory of the user from where you want to access the cluster.
Once the cluster is created, a file named admin.conf will be generated in the `/etc/kubernetes` directory. This file has to be copied to the home directory of the target user.
Execute the below commands on the master node from the non-root user to access the cluster from that respective user:
```
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

5. Deploy network CNI plugin:
```
kubectl apply -f https://docs.projectcalico.org/v3.8/manifests/calico.yaml
```

### OpenEBS ZFS CSI Driver Setup:

1. Prerequisites:

Before installing ZFS driver please make sure your Kubernetes Cluster must meet the following prerequisites:

* all the nodes must have zfs utils installed
* ZPOOL has been setup for provisioning the volume
* You have access to install RBAC components into the kube-system namespace. The OpenEBS ZFS driver components are installed in the kube-system namespace to allow them to be flagged as system critical components.

2. Nodes setup:

All the nodes should have `zfsutils-linux` installed. We should go to the each node of the cluster and install zfs utils

```
$ apt-get install zfsutils-linux
````

Go to each node and create the ZFS Pool, which will be used for provisioning the volumes. You can use the disk in each VM (say /dev/sdb) to create a stripped pool using the below command.
```
zpool create zfspv-pool /dev/sdb
```
3. CSI driver installation:

You can install the latest release of OpenEBS ZFS driver by running the following command in the master node:

```
$ kubectl apply -f https://openebs.github.io/charts/zfs-operator.yaml
```

		
4. Verify that the ZFS driver Components are installed and running using below command :
		
```
$ kubectl get pods -n kube-system -l role=openebs-zfs
```

# Setting up the dev environment (the workstation)

1. Accessing Cluster from a workstation: 
In order to allow access the cluster from a workstation outside the kubernetes cluster you need to:

Make sure that the your workstation can communicate with the control plane
```
ping @master_node` and you can ssh into it
```

Install the client `kubectl` to communicate with the API server
```
sudo apt-get install -y kubectl
```

Copy the Kubernetes credentials file from the master node via ssh
```
scp username@controlplane:/home/username/.kube /home/workstation_user/.kube
```

2. Tools installation:
Install go:
```
$ curl -OL https://golang.org/dl/go1.20.5.linux-amd64.tar.gz
$ sudo tar -C /usr/local -xvf go1.20.5.linux-amd64.tar.gz
```

Install Kubebuilder:
```
$ curl -L -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
$ chmod +x kubebuilder && mv kubebuilder /usr/local/bin/
```

3. Clone this github repo

# Tests
Make sure to have the kubeconfig file on your workstation machine because the controller will automatically use the current context in your kubeconfig file.

1. The snapshotting module:

Apply the testing pod, pv and pvc
```
kubectl apply -f /path/to/the/project/test-manifests/test-openebszfs
```
Apply the MigrationRequset CRD at
```
Kubectl apply -f  /path/to/the/project/config/crd/bases/api.k8s.zfs-volume-migrator.io_migrationrequests.yaml
```
Apply the sample MigrationRequset object:
```
Kubectl apply -f /path/to/the/project/config/samples/api_v1_migrationrequest.yaml
```
Run the controller in vscode and watch for the creation of A VolumeSnapshotClass named "migration-vsc" and a VolumeSnapshot for the testing PV. At this point a VolumeSnapshotContent is created and the actual snapshot of the volume on disk is captured.
