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

	openebszfsv1 "github.com/openebs/zfs-localpv/pkg/apis/openebs.io/zfs/v1"
	apiv1 "github.com/thehamdiaz/first-controller.git/api/v1"
	v1 "github.com/thehamdiaz/first-controller.git/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RestoreRequestReconciler reconciles a RestoreRequest object
type RestoreRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=restorerequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=restorerequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.k8s.zfs-volume-migrator.io,resources=restorerequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RestoreRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
// Reconcile implements the reconciliation loop for the RestoreRequest CRD
func (r *RestoreRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the RestoreRequest object
	restoreReq := &apiv1.RestoreRequest{}
	if err := r.Get(ctx, req.NamespacedName, restoreReq); err != nil {
		if errors.IsNotFound(err) {
			// Object not found Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the RestoreRequest is already completed
	if restoreReq.Status.Succeeded == "True" {
		log.Info("RestoreRequest is already completed")
		return ctrl.Result{}, nil
	}

	// Create PV
	err := CreatePV(ctx, r.Client, restoreReq)
	if err != nil {
		restoreReq.Status.Succeeded = "False"
		restoreReq.Status.Message = fmt.Sprintf("Failed to create PV: %v", err)
		if updateErr := r.Status().Update(ctx, restoreReq); updateErr != nil {
			log.Error(updateErr, "Failed to update RestoreRequest status")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Create PVC
	err = CreatePVC(ctx, r.Client, restoreReq)
	if err != nil {
		restoreReq.Status.Succeeded = "False"
		restoreReq.Status.Message = fmt.Sprintf("Failed to create PVC: %v", err)
		if updateErr := r.Status().Update(ctx, restoreReq); updateErr != nil {
			log.Error(updateErr, "Failed to update RestoreRequest status")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Create ZFSVolume
	err = CreateZFSVolume(ctx, r.Client, restoreReq)
	if err != nil {
		restoreReq.Status.Succeeded = "False"
		restoreReq.Status.Message = fmt.Sprintf("Failed to create ZFSVolume: %v", err)
		if updateErr := r.Status().Update(ctx, restoreReq); updateErr != nil {
			log.Error(updateErr, "Failed to update RestoreRequest status")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Update RestoreRequest status
	restoreReq.Status.Succeeded = "True"
	restoreReq.Status.Message = "RestoreRequest completed successfully"
	if err := r.Status().Update(ctx, restoreReq); err != nil {
		log.Error(err, "Failed to update RestoreRequest status")
		return ctrl.Result{}, err
	}

	log.Info("RestoreRequest reconciliation completed")
	return ctrl.Result{}, nil
}

func CreatePV(ctx context.Context, k8sClient client.Client, restoreReq *v1.RestoreRequest) error {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: restoreReq.Spec.PVName,
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: restoreReq.Spec.StorageClassName,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: restoreReq.Spec.Capacity,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(restoreReq.Spec.AccessModes[0]),
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimPolicy(restoreReq.Spec.ReclaimPolicy),
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "zfs.csi.openebs.io",
					VolumeHandle: restoreReq.Spec.ZFSDatasetName,
					VolumeAttributes: map[string]string{
						"datasetName": restoreReq.Spec.ZFSDatasetName,
					},
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/hostname",
									Operator: corev1.NodeSelectorOpIn,
									Values: []string{
										restoreReq.Spec.TargetNodeName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, pv, &client.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PV: %v", err)
	}

	fmt.Printf("Persistent Volume %s created\n", pv.ObjectMeta.Name)
	return nil
}

func CreatePVC(ctx context.Context, k8sClient client.Client, restoreReq *apiv1.RestoreRequest) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreReq.Spec.PVCName,
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &restoreReq.Spec.StorageClassName,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode(restoreReq.Spec.AccessModes[0]),
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: restoreReq.Spec.Capacity,
				},
			},
			VolumeName: restoreReq.Spec.PVName,
		},
	}

	if err := k8sClient.Create(ctx, pvc, &client.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PVC: %v", err)
	}

	fmt.Printf("Persistent Volume Claim %s created\n", pvc.ObjectMeta.Name)
	return nil
}

func CreateZFSVolume(ctx context.Context, k8sClient client.Client, rr *apiv1.RestoreRequest) error {
	// Convert the capacity from GB to bytes
	capacityInBytesString := fmt.Sprintf("%d", rr.Spec.Capacity.Value()*1024*1024*1024)

	// Create the ZFSVolume object
	zfsVolume := &openebszfsv1.ZFSVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rr.Spec.ZFSDatasetName,
			Namespace: "openebs",
			Labels: map[string]string{
				"kubernetes.io/nodename": rr.Spec.TargetNodeName,
			},
			Finalizers: []string{"zfs.openebs.io/finalizer"},
		},
		Spec: openebszfsv1.VolumeInfo{
			Capacity:    capacityInBytesString,
			Compression: "on",
			Dedup:       "off",
			FsType:      "zfs",
			OwnerNodeID: rr.Spec.TargetNodeName,
			PoolName:    rr.Spec.ZFSPoolName,
			VolumeType:  "DATASET",
		},
		Status: openebszfsv1.VolStatus{
			State: "Ready",
		},
	}

	// Create the ZFSVolume object need to writet the status also
	if err := k8sClient.Create(ctx, zfsVolume, &client.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create ZFSVolume: %v", err)
	}

	fmt.Printf("ZFSVolume %s created\n", zfsVolume.ObjectMeta.Name)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.RestoreRequest{}).
		Complete(r)
}
