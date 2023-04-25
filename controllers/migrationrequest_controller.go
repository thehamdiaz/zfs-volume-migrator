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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	apiv1 "github.com/thehamdiaz/first-controller.git/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigrationRequestReconciler reconciles a MigrationRequest object
type MigrationRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
	var migrationRequest apiv1.MigrationRequest
	if err := r.Get(ctx, req.NamespacedName, &migrationRequest); err != nil {
		l.Error(err, "unable to fetch MigrationRequest")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the pod
	var pod corev1.Pod
	if err := r.Get(ctx, types.NamespacedName{Namespace: migrationRequest.Namespace, Name: migrationRequest.Spec.PodName}, &pod); err != nil {
		l.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}

	// Fetch the PVC
	var pvc corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName}, &pvc); err != nil {
		l.Error(err, "unable to fetch PVC")
		return ctrl.Result{}, err
	}

	// Create the VolumeSnapshotClass
	/*vsc := &snapv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "migration-vsc",
			Namespace: migrationRequest.Namespace,
		},
		Driver:         "zfs.csi.openebs.io",
		DeletionPolicy: "Delete",
		Parameters: map[string]string{
			"snapshotNamePrefix": "migration-snapshot-",
		},
	}

	if err := r.Create(ctx, vsc); err != nil {
		l.Error(err, "unable to create VolumeSnapshotClass")
		return ctrl.Result{}, err
	}*/

	classname := "migration-vsc"
	// Create the VolumeSnapshot
	vs := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "migration-snapshot-",
			Namespace:    migrationRequest.Namespace,
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvc.Name,
			},
			VolumeSnapshotClassName: &classname,
		},
	}

	if err := r.Create(ctx, vs); err != nil {
		l.Error(err, "unable to create VolumeSnapshot")
		return ctrl.Result{}, err
	}

	// Poll the status of the VolumeSnapshot until it is "ReadyToUse"
	for {
		var snapshot snapv1.VolumeSnapshot
		if err := r.Get(ctx, types.NamespacedName{Namespace: vs.Namespace, Name: vs.Name}, &snapshot); err != nil {
			l.Error(err, "unable to fetch VolumeSnapshot")
			return ctrl.Result{}, err
		}

		if snapshot.Status.ReadyToUse != nil && *snapshot.Status.ReadyToUse == true {
			break
		}

		time.Sleep(2 * time.Second)
	}

	// Update the status of the MigrationRequest object
	migrationRequest.Status.SnapshotCreated = "True"
	if err := r.Status().Update(ctx, &migrationRequest); err != nil {
		l.Error(err, "unable to update MigrationRequest status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.MigrationRequest{}).
		Complete(r)
}
