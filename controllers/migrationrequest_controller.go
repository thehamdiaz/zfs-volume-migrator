/*
Copyright 2023 thehamdiaz.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	apiv1 "github.com/thehamdiaz/first-controller.git/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	//"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// MigrationRequestReconciler reconciles a MigrationRequest object
type MigrationRequestReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	CachedData map[string]CachedResources
}

type CachedResources struct {
	Pod                   *corev1.Pod
	PersistentVolume      *corev1.PersistentVolume
	PersistentVolumeClaim *corev1.PersistentVolumeClaim
	VolumeSnapshotClass   *snapv1.VolumeSnapshotClass
	ConfigMap             *corev1.ConfigMap
	Secret                *corev1.Secret
}

//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=migrationrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=migrationrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=migrationrequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MigrationRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MigrationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the MigrationRequest object
	migrationRequest := &apiv1.MigrationRequest{}
	if err := r.Get(ctx, req.NamespacedName, migrationRequest); err != nil {
		if errors.IsNotFound(err) {
			// Object not found Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the RestoreRequest is already completed (This field is set by the restore Controller)
	if migrationRequest.Status.MigrationCompleted == "True" {
		l.Info("MigrationRequest is already completed")
		return ctrl.Result{}, nil
	}
	// Check if all the snapshots have been sent (sending completed)
	if migrationRequest.Status.AllSnapshotsSent == "True" {
		l.Info("All the snapshots have been sent")
		return ctrl.Result{}, nil
	}

	// Retrieve or use the cached data
	cachedData, exists := r.CachedData[migrationRequest.Name]
	if !exists {
		// Cached data doesn't exist, fetch or create it
		// Fetch the Pod
		if err := r.Get(ctx, types.NamespacedName{Namespace: migrationRequest.Namespace, Name: migrationRequest.Spec.PodName}, cachedData.Pod); err != nil {
			l.Error(err, "unable to fetch Pod")
			return ctrl.Result{}, err
		}
		// Fetch the PersistentVolumeClaim
		if err := r.Get(ctx, types.NamespacedName{Namespace: cachedData.Pod.Namespace, Name: cachedData.Pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName}, cachedData.PersistentVolumeClaim); err != nil {
			l.Error(err, "unable to fetch PVC")
			return ctrl.Result{}, err
		}
		// Fetch the PersistentVolume
		if err := r.Get(ctx, types.NamespacedName{Namespace: cachedData.Pod.Namespace, Name: cachedData.PersistentVolumeClaim.Spec.VolumeName}, cachedData.PersistentVolume); err != nil {
			l.Error(err, "unable to fetch PV")
			return ctrl.Result{}, err
		}
		// Create the VolumeSnapshotClass
		var err error
		cachedData.VolumeSnapshotClass, err = r.ensureVolumeSnapshotClass(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create VolumeSnapshotClass")
			return ctrl.Result{}, err
		}
		// Create the ConfigMap
		cachedData.ConfigMap, err = r.createConfigMapObject(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the ConfigMap")
			return ctrl.Result{}, err
		}

		cachedData.Secret, err = r.createSecretObject(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the Secret")
			return ctrl.Result{}, err
		}

	}
	// stop condition (can and will be modified)
	if migrationRequest.Spec.DesiredSnapshotCount != migrationRequest.Status.SnapshotCount {
		snapshot, err := r.createAndEnsureVolumeSnapshotReadiness(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the Volumesnapshot")
			return ctrl.Result{}, err
		}
		err = r.sendSnapshot(ctx, migrationRequest, snapshot)
		if err != nil {
			l.Error(err, "failed to send snapshot")
			return ctrl.Result{}, err
		}

		// incriment the number of snapshots
		migrationRequest.Status.SnapshotCount++

		// Wait for the specified interval
		time.Sleep(time.Duration(migrationRequest.Spec.SnapInterval))

		// Enqueue the resource for the next reconciliation
		return ctrl.Result{Requeue: true}, nil
	} else {
		err := r.stopPod(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "failed to stop the pod")
			return ctrl.Result{}, err
		}
		snapshot, err := r.createAndEnsureVolumeSnapshotReadiness(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the Volumesnapshot")
			return ctrl.Result{}, err
		}
		err = r.sendSnapshot(ctx, migrationRequest, snapshot)
		if err != nil {
			l.Error(err, "failed to send snapshot")
			return ctrl.Result{}, err
		}
		// this will be created in the remote node trigerring the restoring controller
		_, err = r.createRestoreRequest(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "failed to create restoreRequest")
			return ctrl.Result{}, err
		}
		migrationRequest.Status.AllSnapshotsSent = "True"
	}
	/*
		// Update the status of the MigrationRequest object
		migrationRequest.Status.SnapshotCreated = "True"
		if err := r.Status().Update(ctx, migrationRequest); err != nil {
			l.Error(err, "unable to update MigrationRequest status")
			return ctrl.Result{}, err
		}*/

	// Create RestoreRequest object for testing
	/*restoreReqRef, _ := createRestoreRequestObject(&pv, &pvc, "restoredPv", "restoredPvc", "worker1")
	if err := r.Create(ctx, restoreReqRef); err != nil {
		l.Error(err, "unable to create the RestoreRequest object")
		return ctrl.Result{}, err
	}*/

	/*//get the capacity
	quantity, _ := resource.ParseQuantity(pv.Spec.Capacity.Storage().String())
	// Create a new RestoreRequest object and set its fields
	restoreReqRef := &apiv1.RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "restore-", // Provide a unique name for the object
			Namespace:    "default",
		},

		Spec: apiv1.RestoreRequestSpec{
			Capacity:         quantity,
			AccessModes:      pv.Spec.AccessModes,
			ReclaimPolicy:    pv.Spec.PersistentVolumeReclaimPolicy,
			StorageClassName: pv.Spec.StorageClassName,
			PVName:           "restored-pv",
			PVCName:          "restored-pvc",
			PVCResources:     pvc.Spec.Resources,
			ZFSDatasetName:   "exdataset",
			ZFSPoolName:      "zfspv-pool",
			TargetNodeName:   "worker1",
		},
	}

	if err := r.Create(ctx, restoreReqRef); err != nil {
		l.Error(err, "unable to create the RestoreRequest object")
		return ctrl.Result{}, err
	}
	*/
	return ctrl.Result{}, nil
}

func (r *MigrationRequestReconciler) createRestoreRequest(ctx context.Context, migrationRequest *apiv1.MigrationRequest) (*apiv1.RestoreRequest, error) {
	// Get the capacity
	quantity, _ := resource.ParseQuantity(r.CachedData[migrationRequest.Name].PersistentVolume.Spec.Capacity.Storage().String())

	// Create a new RestoreRequest object and set its fields
	restoreReq := &apiv1.RestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "restore-", // Provide a unique name for the object
			Namespace:    "default",
		},
		Spec: apiv1.RestoreRequestSpec{
			Names: apiv1.Names{
				MigrationRequestName: migrationRequest.Name,
				StorageClassName:     r.CachedData[migrationRequest.Name].PersistentVolume.Spec.StorageClassName,
				PVName:               "restored-" + r.CachedData[migrationRequest.Name].PersistentVolume.Name,
				PVCName:              "restored-" + r.CachedData[migrationRequest.Name].PersistentVolumeClaim.Name,
				ZFSDatasetName:       r.CachedData[migrationRequest.Name].ConfigMap.Data["remoteDataset"],
				ZFSPoolName:          r.CachedData[migrationRequest.Name].ConfigMap.Data["remotePool"],
				TargetNodeName:       r.CachedData[migrationRequest.Name].ConfigMap.Data["remoteHost"],
			},
			Parameters: apiv1.Parameters{
				Capacity:      quantity,
				AccessModes:   r.CachedData[migrationRequest.Name].PersistentVolume.Spec.AccessModes,
				ReclaimPolicy: r.CachedData[migrationRequest.Name].PersistentVolume.Spec.PersistentVolumeReclaimPolicy,
				PVCResources:  r.CachedData[migrationRequest.Name].PersistentVolumeClaim.Spec.Resources,
			},
		},
	}

	if err := r.Create(ctx, restoreReq); err != nil {
		return nil, err
	}

	return restoreReq, nil
}

func (r *MigrationRequestReconciler) ensureVolumeSnapshotClass(ctx context.Context, migrationRequest *apiv1.MigrationRequest) (*snapv1.VolumeSnapshotClass, error) {
	vscName := migrationRequest.Spec.VolumeSnapshotClassName //"migration-vsc"
	vscNamespace := migrationRequest.Namespace
	existingVSC := &snapv1.VolumeSnapshotClass{}
	err := r.Get(ctx, types.NamespacedName{Name: vscName, Namespace: vscNamespace}, existingVSC)
	if err == nil {
		// VolumeSnapshotClass already exists, return the existing one
		return existingVSC, nil
	}

	// Create the VolumeSnapshotClass
	vsc := &snapv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vscName,
			Namespace: vscNamespace,
		},
		Driver:         "zfs.csi.openebs.io",
		DeletionPolicy: "Delete",
		Parameters: map[string]string{
			"snapshotNamePrefix": "migration-snapshot-",
		},
	}

	if err := r.Create(ctx, vsc); err != nil {
		return nil, err
	}

	return vsc, nil
}

func (r *MigrationRequestReconciler) createConfigMapObject(ctx context.Context, migrationRequest *apiv1.MigrationRequest) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "snapshot-migration-config-",
		},
		Data: map[string]string{
			"PREVIOUS":      "None",
			"SNAPSHOT":      "None",
			"USER":          migrationRequest.Spec.Destination.User,
			"REMOTEPOOL":    migrationRequest.Spec.Destination.RemotePool,
			"REMOTEDATASET": migrationRequest.Spec.Destination.RemoteDataset,
			"REMOTEHOST":    migrationRequest.Spec.Destination.RemoteHost,
		},
	}

	err := r.Create(ctx, configMap)
	if err != nil {
		// Handle the error
		return nil, err
	}

	return configMap, nil
}

func (r *MigrationRequestReconciler) createSecretObject(ctx context.Context, migrationRequest *apiv1.MigrationRequest) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "ssh-keys-secret",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"id_rsa":     []byte("LS0tLS1CRUdJTiBPUEVOU1NIIFBSSVZBVEUgS0VZLS0tLS0KYjNCbGJuTnphQzFyWlhrdGRqRUFBQUFBQkc1dmJtVUFBQUFFYm05dVpRQUFBQUFBQUFBQkFBQUJsd0FBQUFkemMyZ3RjbgpOaEFBQUFBd0VBQVFBQUFZRUFvNGxDT3JqTnVyQTdhbmx5bkNUYmNMUE5jSlE0K2dhWkhtNmNsbmVPMnRSRkx2WkQ0S1JlCnRtcmJ1Mng4OUNOaXdMNHZsNGI5dkxaelRueGNRQml5ZmkrTmFLdWZCZGZBVVBNTVFTWWtreGRsTlFFcWVGY1Z6UndFUWIKODAxUlpDaVBWVHEyZUVXZDVLVDMvMENWRldaWkdzNVdqZC8rS3RzdjBaRnE4djBGaE5mcnVnWEtaaUxQNVRrdHE5MkZzagoyOTQ3c29BZ2xKV2J1UnNKNXFRWnpUdXdodTRJcWFzNFYwdFRLT05FaGlNYit2cWcvdjlwK2lIa2tUeGU3Y3A5RDN5UGlyCmltSDNQb1BHRlZ0MTZNS2VjOHEvMnZnK2FhUDhMcWlBZmVaWm4rNzFPdmhUb0RXdXp6Vkt4MktScjgxbTFwZHFoVml1eVEKaXhSeFhOaWNmUm5iRkRsWERxQlFYb2FmUnZFUmVrR1VVNFFtSU50VVBKTjcxc1FydTF2dmlGakxXenBxbm5iN3phYkxjWAoyNUxvSjA1RGFrNFdoTGMrR1UxUFYyMmw0Z2RlSkZlTEVFaGNmQnhlUUphMFpTbjB2aUp6WFZ6WnN2N3FyZlRLV1ZmWXZXCmx2VmtGdUk5MXl2VmRQTDdJWHZOb1EvUzlDUGhYbFhzakN3WjNNNFBBQUFGaU1YcHVBWEY2YmdGQUFBQUIzTnphQzF5YzIKRUFBQUdCQUtPSlFqcTR6YnF3TzJwNWNwd2syM0N6elhDVU9Qb0dtUjV1bkpaM2p0clVSUzcyUStDa1hyWnEyN3RzZlBRagpZc0MrTDVlRy9ieTJjMDU4WEVBWXNuNHZqV2lybndYWHdGRHpERUVtSkpNWFpUVUJLbmhYRmMwY0JFRy9OTlVXUW9qMVU2CnRuaEZuZVNrOS85QWxSVm1XUnJPVm8zZi9pcmJMOUdSYXZMOUJZVFg2N29GeW1ZaXorVTVMYXZkaGJJOXZlTzdLQUlKU1YKbTdrYkNlYWtHYzA3c0lidUNLbXJPRmRMVXlqalJJWWpHL3I2b1A3L2Fmb2g1SkU4WHUzS2ZROThqNHE0cGg5ejZEeGhWYgpkZWpDbm5QS3Y5cjRQbW1qL0M2b2dIM21XWi91OVRyNFU2QTFyczgxU3NkaWthL05adGFYYW9WWXJza0lzVWNWelluSDBaCjJ4UTVWdzZnVUY2R24wYnhFWHBCbEZPRUppRGJWRHlUZTliRUs3dGI3NGhZeTFzNmFwNTIrODJteTNGOXVTNkNkT1EycE8KRm9TM1BobE5UMWR0cGVJSFhpUlhpeEJJWEh3Y1hrQ1d0R1VwOUw0aWMxMWMyYkwrNnEzMHlsbFgyTDFwYjFaQmJpUGRjcgoxWFR5K3lGN3phRVAwdlFqNFY1VjdJd3NHZHpPRHdBQUFBTUJBQUVBQUFHQUptSmtzN0RGeFJCeFdidjR6VEtQZVNRU3o5CjVTZzBrQ0xwVHExeHhuNFBBYTd2dHBralF5Y09HakFwcGp0OUFJY1ZJU2pKL29OWitqYitRYnFRWEMrNEJBMGpVYUpiNXUKeXZGSlNvOWYzVkNMOWtWNFNQZXp5OGxNTEh4ck02cStZalFtOTkvYnZsWkJIZWpjQ0VYWm9BeHh4d1QydW9Wam5OUHdUQgpWQmhVYjhwWWIzakZlWFNwVkZXMzVST2hPbVZvaVNmWUs2WXZXOHI5VnJYUUhlZG9BUW5wTUhZSCtxUVQ4U1hWSCt0dmROCnJYcWZTRXI5L25KdkdqSFAzRU4rOS90R2pwTzI3TEMxdlQxREo3eS9wOTBSK0JESWZHTjBJQWQ5RVVtdFVPY3p5ckMxWGkKRm9OTzhFT1dHRzhIY25xRld2a0hJdlVSRGc4Z1Y2S1psU3hJZXlXSzhDdEpNdmM4ZVdFdHQ5UnlWYlhtdFJPcEhlRlBqeQpCOWdvZGJSY1g0dExQdUE0TEIxbTRWaDlFK1pHbG1DcDQramhFQUJBelF6dmNzY2JkUnJZZHRsbWhwN1ZEQ3JRbE9OZ0FaCkRYNU5xM1JkUnBtVkNRbmlIRm1TTm1wQjhSR1VHRHl2U2k4WXBrRGFmWVVtUEw3dkQ5Yk51Tnl4NktYMjl5VWwrUkFBQUEKd1FDb3hTdjNiS2RtK04zWHkwaFNmc1NvMDVvTlNCNHBOOExQNVY5WlNLRzFrZlJpNWk0blJFY3N1bWo2WDVjRXQ4SytWNApHV0Q3OXRqc1crcDMwVUlKZUV5aVRDN2ZqQStDeWdoYThnd1U1Y1gvSDQyMEtWWWhpL1B6TXhHWUppUVhxbGZnSG52bXE2CkpVL1M5emRJOU1DcDMzeGlrWjE4dXg2ZVFBYVhxNS9YMEdUSEhkblJBZG0vWGFIYUs5Umt6bEErdy9xTlRtR1czcEJySG8KU0hNc2F3ZDd0Sit3cWRnd1NjS2ozaEgxbGhhTEFZRnd4czIrK3lvUkpKb3BhVFFJZ0FBQURCQU1QVkF5ZWhYcEtkQWtUeQpIVjdoSEg4a0RGYXBuQ0x4OEd1czY0emFjZFROZ2ZDSGZWcjhYV0U3MG9HN1dIVDBVWDNnck5TdmpCdmR6WTdpK1dTZjVYCll2eEptRnljeE0yR3BXV2ZvdHdqRUgvd0dCemhua0hFU2VkOWo0WERjZ0JFWU1GdVYrUjM4aFYzRXRLcE5mdnlDK1pqZDUKQll6Ym1IMHNtdVcwWDZhQ0ZhTktUeTBZL1ZiMXJhNVN1RURiNFVLWGI2V1BNSmNodTZWdDBRKzVIUktRRG9rdjRHMGVzRQpjWkQ3NDVmYnVseHNESUlaaXBYQlN0WWVNcEFPNkFQd0FBQU1FQTFjZ0t0Rm5JTkhkMjJ1cjVVa0pROWN4OGZwYTZLalpBCkc4M1JHaUxuaUw1ZE1aVGlaSk5tZ3p5WXBuSE1GT3lrMk9YTzNMVG1DYWxJL1pGSWxuY0l3bjEvdWlUbmZmMlBoUm5Ec3UKL3M2VVdHSE84TmtDdmprZU8xSXFuNjU3L21QQTVWUjZMb0hKSVlZUHFoNG5yVko3K2toVTFsUkc5REhUaTVtYXVkTDZUZApGaDBWK1RtbnVRVTl0Z2NsYmtUWEdmbk5TaFJMVlJPdDhLVW1PSlNBdnp0TktXbVpIN1Y5N3NkdVhhNkR2akFmY2Nqc0lkCkpkVFQ5ZTd0aWRHVDR4QUFBQURISnZiM1JBZW1aekxYQnZaQUVDQXdRRkJnPT0KLS0tLS1FTkQgT1BFTlNTSCBQUklWQVRFIEtFWS0tLS0tCg"),
			"id_rsa.pub": []byte("c3NoLXJzYSBBQUFBQjNOemFDMXljMkVBQUFBREFRQUJBQUFCZ1FDamlVSTZ1TTI2c0R0cWVYS2NKTnR3czgxd2xEajZCcGtlYnB5V2Q0N2ExRVV1OWtQZ3BGNjJhdHU3Ykh6MEkyTEF2aStYaHYyOHRuTk9mRnhBR0xKK0w0MW9xNThGMThCUTh3eEJKaVNURjJVMUFTcDRWeFhOSEFSQnZ6VFZGa0tJOVZPclo0Uloza3BQZi9RSlVWWmxrYXpsYU4zLzRxMnkvUmtXcnkvUVdFMSt1NkJjcG1Jcy9sT1MycjNZV3lQYjNqdXlnQ0NVbFp1NUd3bm1wQm5OTzdDRzdnaXBxemhYUzFNbzQwU0dJeHY2K3FEKy8ybjZJZVNSUEY3dHluMFBmSStLdUtZZmMrZzhZVlczWG93cDV6eXIvYStENXBvL3d1cUlCOTVsbWY3dlU2K0ZPZ05hN1BOVXJIWXBHdnpXYldsMnFGV0s3SkNMRkhGYzJKeDlHZHNVT1ZjT29GQmVocDlHOFJGNlFaUlRoQ1lnMjFROGszdld4Q3U3VysrSVdNdGJPbXFlZHZ2TnBzdHhmYmt1Z25Ua05xVGhhRXR6NFpUVTlYYmFYaUIxNGtWNHNRU0Z4OEhGNUFsclJsS2ZTK0luTmRYTm15L3VxdDlNcFpWOWk5YVc5V1FXNGozWEs5VjA4dnNoZTgyaEQ5TDBJK0ZlVmV5TUxCbmN6Zzg9IHJvb3RAemZzLXBvZAo"),
		},
	}

	err := r.Create(ctx, secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

func (r *MigrationRequestReconciler) createAndEnsureVolumeSnapshotReadiness(ctx context.Context, migrationRequest *apiv1.MigrationRequest) (*snapv1.VolumeSnapshot, error) {
	vs := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "migration-snapshot-",
			Namespace:    migrationRequest.Namespace,
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &r.CachedData[migrationRequest.Name].PersistentVolumeClaim.Name,
			},
			VolumeSnapshotClassName: &r.CachedData[migrationRequest.Name].VolumeSnapshotClass.Name,
		},
	}

	if err := r.Create(ctx, vs); err != nil {
		return nil, err
	}

	for {
		var snapshot snapv1.VolumeSnapshot
		if err := r.Get(ctx, types.NamespacedName{Namespace: vs.Namespace, Name: vs.Name}, &snapshot); err != nil {
			return nil, err
		}

		if snapshot.Status != nil && snapshot.Status.ReadyToUse != nil && *snapshot.Status.ReadyToUse {
			return &snapshot, nil
		}

		time.Sleep(time.Second)
	}
}

func (r *MigrationRequestReconciler) sendSnapshot(ctx context.Context, migrationRequest *apiv1.MigrationRequest, vs *snapv1.VolumeSnapshot) error {
	// Fetch the VolumeSnapshotContent
	var vsContent snapv1.VolumeSnapshotContent
	err := r.Get(ctx, types.NamespacedName{Name: *vs.Status.BoundVolumeSnapshotContentName}, &vsContent)
	if err != nil {
		return err
	}

	// Modify the ConfigMap
	r.CachedData[migrationRequest.Name].ConfigMap.Data["PREVIOUS"] = r.CachedData[migrationRequest.Name].ConfigMap.Data["SNAPSHOT"]
	r.CachedData[migrationRequest.Name].ConfigMap.Data["SNAPSHOT"] = *vsContent.Status.SnapshotHandle

	// Apply the updated ConfigMap
	err = r.Update(ctx, r.CachedData[migrationRequest.Name].ConfigMap)
	if err != nil {
		return err
	}

	// Create the Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "snapshot-migration-sender-append-" + r.CachedData[migrationRequest.Name].PersistentVolume.Name,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "zfs-container",
							Image: "thehamdiaz/zfs-ubuntu:v10.0",
							SecurityContext: &corev1.SecurityContext{
								Privileged: func() *bool { b := true; return &b }(),
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: r.CachedData[migrationRequest.Name].ConfigMap.Name,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ssh-keys",
									MountPath: "/etc/ssh-key",
									ReadOnly:  true,
								},
							},
						},
					},
					RestartPolicy: "Never",
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": r.CachedData[migrationRequest.Name].Pod.Spec.NodeName,
					},
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{r.CachedData[migrationRequest.Name].Pod.Spec.NodeName},
											},
										},
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "ssh-keys",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: r.CachedData[migrationRequest.Name].Secret.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	// Create the Job
	err = r.Create(ctx, job)
	if err != nil {
		return err
	}

	// Wait for the Job to finish
	jobKey := types.NamespacedName{Name: job.Name, Namespace: job.Namespace}
	err = wait.PollImmediate(time.Second, time.Minute*5, func() (bool, error) {
		if err := r.Get(ctx, jobKey, job); err != nil {
			return false, err
		}

		if job.Status.CompletionTime != nil {
			if job.Status.Succeeded > 0 {
				return true, nil
			}
			return false, fmt.Errorf("job failed: %s", job.Status.String())
		}

		return false, nil
	})

	return err
}

func (r *MigrationRequestReconciler) stopPod(ctx context.Context, migrationRequest *apiv1.MigrationRequest) error {
	pod := r.CachedData[migrationRequest.Name].Pod

	// Set the pod's deletion timestamp to stop it
	gracePeriodSeconds := int64(0)
	deleteOptions := client.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	err := r.Delete(ctx, pod, &deleteOptions)
	if err != nil {
		return err
	}

	// Wait until the pod is deleted
	for {
		err = r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			if errors.IsNotFound(err) {
				// Pod is deleted
				break
			}
			return err
		}

		time.Sleep(time.Second)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.MigrationRequest{}).
		Complete(r)
}
