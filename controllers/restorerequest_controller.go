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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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
	err := r.createPV(ctx, restoreReq)
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
	err = r.createPVC(ctx, restoreReq)
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
	err = r.createZFSVolume(ctx, restoreReq)
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

	// Update migrationRequest object status in the source cluster
	if err := r.updateMigrationRequestStatus(ctx, restoreReq); err != nil {
		log.Error(err, "Failed to update migrationRequest status")
		return ctrl.Result{}, err
	}

	log.Info("RestoreRequest reconciliation completed")
	return ctrl.Result{}, nil
}

func (r *RestoreRequestReconciler) createPV(ctx context.Context, restoreRequest *apiv1.RestoreRequest) error {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: restoreRequest.Spec.Names.PVName,
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: restoreRequest.Spec.Names.StorageClassName,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: restoreRequest.Spec.Parameters.Capacity,
			},
			AccessModes:                   make([]corev1.PersistentVolumeAccessMode, len(restoreRequest.Spec.Parameters.AccessModes)),
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimPolicy(restoreRequest.Spec.Parameters.ReclaimPolicy),
			ClaimRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Name:       restoreRequest.Spec.Names.PVCName,
				Namespace:  "default",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					FSType:       "zfs",
					Driver:       "zfs.csi.openebs.io",
					VolumeHandle: restoreRequest.Spec.Names.ZFSDatasetName,
					VolumeAttributes: map[string]string{
						"openebs.io/poolname": restoreRequest.Spec.Names.ZFSPoolName,
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
										restoreRequest.Spec.Names.TargetNodeName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Populate the AccessModes field
	for i, mode := range restoreRequest.Spec.Parameters.AccessModes {
		pv.Spec.AccessModes[i] = mode
	}

	// Create the PV
	if err := r.Create(ctx, pv); err != nil {
		return fmt.Errorf("failed to create PV: %v", err)
	}

	fmt.Printf("Persistent Volume %s created\n", pv.ObjectMeta.Name)
	return nil
}

func (r *RestoreRequestReconciler) createPVC(ctx context.Context, restoreRequest *apiv1.RestoreRequest) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreRequest.Spec.Names.PVCName,
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &restoreRequest.Spec.Names.StorageClassName,
			AccessModes:      make([]corev1.PersistentVolumeAccessMode, len(restoreRequest.Spec.Parameters.AccessModes)),
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: restoreRequest.Spec.Parameters.Capacity,
				},
			},
		},
	}

	// Populate the AccessModes field
	for i, mode := range restoreRequest.Spec.Parameters.AccessModes {
		pvc.Spec.AccessModes[i] = mode
	}

	// Create the PVC
	if err := r.Create(ctx, pvc); err != nil {
		return fmt.Errorf("failed to create PVC: %v", err)
	}

	fmt.Printf("Persistent Volume Claim %s created\n", pvc.ObjectMeta.Name)
	return nil
}

func (r *RestoreRequestReconciler) createZFSVolume(ctx context.Context, restoreRequest *apiv1.RestoreRequest) error {
	// Convert the capacity from GB to bytes
	capacityInBytesString := fmt.Sprintf("%d", restoreRequest.Spec.Parameters.Capacity.Value())
	fmt.Printf("Capacity is %s\n", capacityInBytesString)

	// Create the ZFSVolume object
	zfsVolume := &openebszfsv1.ZFSVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:       restoreRequest.Spec.Names.ZFSDatasetName,
			Namespace:  "openebs",
			Finalizers: []string{"zfs.openebs.io/finalizer"},
		},
		Spec: openebszfsv1.VolumeInfo{
			Capacity:    capacityInBytesString,
			Compression: "off",
			Dedup:       "off",
			FsType:      "zfs",
			OwnerNodeID: restoreRequest.Spec.Names.TargetNodeName,
			PoolName:    restoreRequest.Spec.Names.ZFSPoolName,
			VolumeType:  "DATASET",
		},
		Status: openebszfsv1.VolStatus{
			State: "Ready",
		},
	}

	// Create the ZFSVolume object
	if err := r.Create(ctx, zfsVolume); err != nil {
		return fmt.Errorf("failed to create ZFSVolume: %v", err)
	}

	fmt.Printf("ZFSVolume %s created\n", zfsVolume.ObjectMeta.Name)
	return nil
}

func (r *RestoreRequestReconciler) updateMigrationRequestStatus(ctx context.Context, restoreRequest *apiv1.RestoreRequest) error {
	migrationRequestName := restoreRequest.Spec.Names.MigrationRequestName

	// Fetch the associated MigrationRequest object
	migrationRequest := &apiv1.MigrationRequest{}
	err := r.Get(ctx, types.NamespacedName{Name: migrationRequestName, Namespace: restoreRequest.Namespace}, migrationRequest)
	if err != nil {
		return err
	}

	// Update the MigrationRequest Status
	migrationRequest.Status.MigrationCompleted = "true"

	err = r.Status().Update(ctx, migrationRequest)
	if err != nil {
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.RestoreRequest{}).
		Complete(r)
}
