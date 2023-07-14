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
	storagev1 "k8s.io/api/storage/v1"

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
	StorageClass          *storagev1.StorageClass
	VolumeSnapshotClass   *snapv1.VolumeSnapshotClass
	ConfigMap             *corev1.ConfigMap
	Secret                *corev1.Secret
	PreviousSnapshot      *snapv1.VolumeSnapshot
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
	_, exists := r.CachedData[migrationRequest.Name]
	if !exists {
		r.CachedData[migrationRequest.Name] = NewCachedResources()
		cachedResources := r.CachedData[migrationRequest.Name]
		// Cached data doesn't exist, fetch or create it
		// Fetch the Pod
		if err := r.Get(ctx, types.NamespacedName{Namespace: migrationRequest.Namespace, Name: migrationRequest.Spec.PodName}, cachedResources.Pod); err != nil {
			l.Error(err, "unable to fetch Pod")
			return ctrl.Result{}, err
		}
		// Fetch the PersistentVolumeClaim
		if err := r.Get(ctx, types.NamespacedName{Namespace: cachedResources.Pod.Namespace, Name: cachedResources.Pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName}, cachedResources.PersistentVolumeClaim); err != nil {
			l.Error(err, "unable to fetch PVC")
			return ctrl.Result{}, err
		}
		// Fetch the PersistentVolume
		if err := r.Get(ctx, types.NamespacedName{Namespace: cachedResources.Pod.Namespace, Name: cachedResources.PersistentVolumeClaim.Spec.VolumeName}, cachedResources.PersistentVolume); err != nil {
			l.Error(err, "unable to fetch PV")
			return ctrl.Result{}, err
		}
		if err := r.Get(ctx, types.NamespacedName{Name: *cachedResources.PersistentVolumeClaim.Spec.StorageClassName}, cachedResources.StorageClass); err != nil {
			l.Error(err, "unable to fetch Storage class")
			return ctrl.Result{}, err
		}
		// Create the VolumeSnapshotClass
		var err error
		cachedResources.VolumeSnapshotClass, err = r.ensureVolumeSnapshotClass(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create VolumeSnapshotClass")
			return ctrl.Result{}, err
		}
		// Create the ConfigMap
		cachedResources.ConfigMap, err = r.createConfigMapObject(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the ConfigMap")
			return ctrl.Result{}, err
		}

		cachedResources.Secret, err = r.createSecretObject(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "unable to create the Secret")
			return ctrl.Result{}, err
		}
		r.CachedData[migrationRequest.Name] = cachedResources
	}

	// stop condition (can and will be modified)
	if migrationRequest.Status.ConfirmedSnapshotCount < migrationRequest.Spec.DesiredSnapshotCount-1 {
		snapshot := &snapv1.VolumeSnapshot{}
		var err error
		//Previous snapshot wasn't sent
		if migrationRequest.Status.ConfirmedSnapshotCount < migrationRequest.Status.SnapshotCount {
			snapshot = r.CachedData[migrationRequest.Name].PreviousSnapshot
		} else if migrationRequest.Status.ConfirmedSnapshotCount == migrationRequest.Status.SnapshotCount {
			snapshot, err = r.createAndEnsureVolumeSnapshotReadiness(ctx, migrationRequest)
			if err != nil {
				l.Error(err, "unable to create the Volumesnapshot")
				return ctrl.Result{}, err
			}
			// incriment the number of created snapshots
			migrationRequest.Status.SnapshotCount++
			if err = r.Status().Update(ctx, migrationRequest); err != nil {
				l.Error(err, "failed to update migrationRequest status")
				return ctrl.Result{}, err
			}
		}

		err = r.sendSnapshot(ctx, migrationRequest, snapshot)
		if err != nil {
			l.Error(err, "failed to send snapshot")
			// if the send fails requeue this snapshot for the next reconcilation cycle
			CachedResources := r.CachedData[migrationRequest.Name]
			CachedResources.PreviousSnapshot = snapshot
			r.CachedData[migrationRequest.Name] = CachedResources
			return ctrl.Result{}, err
		}

		// if the send succeeded incriment the number of confirmed (sent) snapshots
		migrationRequest.Status.ConfirmedSnapshotCount++
		if err := r.Status().Update(ctx, migrationRequest); err != nil {
			l.Error(err, "failed to update migrationRequest status")
			return ctrl.Result{}, err
		}

		// Wait for the specified interval
		time.Sleep(time.Second * time.Duration(migrationRequest.Spec.SnapInterval))

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
		// At this point all the snapshots are sent
		migrationRequest.Status.AllSnapshotsSent = "True"
		if err := r.Status().Update(ctx, migrationRequest); err != nil {
			l.Error(err, "failed to update migrationRequest status")
			return ctrl.Result{}, err
		}
		// This will be created in the remote node trigerring the restoring controller
		_, err = r.createRestoreRequest(ctx, migrationRequest)
		if err != nil {
			l.Error(err, "failed to create restoreRequest")
			return ctrl.Result{}, err
		}
		/*
			// At this point the migration is completed  this should be executed by the restore cotroller
			migrationRequest.Status.MigrationCompleted = "True"
			if err := r.Status().Update(ctx, migrationRequest); err != nil {
				l.Error(err, "failed to update migrationRequest status")
				return ctrl.Result{}, err
			}
		*/
	}

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
				ZFSDatasetName:       r.CachedData[migrationRequest.Name].ConfigMap.Data["REMOTEDATASET"],
				ZFSPoolName:          r.CachedData[migrationRequest.Name].ConfigMap.Data["REMOTEPOOL"],
				TargetNodeName:       r.CachedData[migrationRequest.Name].ConfigMap.Data["REMOTEHOSTNAME"],
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
			Namespace:    "default",
		},
		Data: map[string]string{
			"PREVIOUS":       "None",
			"SNAPSHOT":       "None",
			"USER":           migrationRequest.Spec.Destination.User,
			"REMOTEPOOL":     migrationRequest.Spec.Destination.RemotePool,
			"REMOTEDATASET":  migrationRequest.Spec.Destination.RemoteDataset,
			"REMOTEHOSTIP":   migrationRequest.Spec.Destination.RemoteHostIP,
			"REMOTEHOSTNAME": migrationRequest.Spec.Destination.RemoteHostName,
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
			GenerateName: "snapshot-migration-secret-",
			Namespace:    "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"id_rsa": []byte(`-----BEGIN OPENSSH PRIVATE KEY-----
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
-----END OPENSSH PRIVATE KEY-----
`),
			"id_rsa.pub": []byte(`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCjiUI6uM26sDtqeXKcJNtws81wlDj6BpkebpyWd47a1EUu9kPgpF62atu7bHz0I2LAvi+Xhv28tnNOfFxAGLJ+L41oq58F18BQ8wxBJiSTF2U1ASp4VxXNHARBvzTVFkKI9VOrZ4RZ3kpPf/QJUVZlkazlaN3/4q2y/RkWry/QWE1+u6BcpmIs/lOS2r3YWyPb3juygCCUlZu5GwnmpBnNO7CG7gipqzhXS1Mo40SGIxv6+qD+/2n6IeSRPF7tyn0PfI+KuKYfc+g8YVW3Xowp5zyr/a+D5po/wuqIB95lmf7vU6+FOgNa7PNUrHYpGvzWbWl2qFWK7JCLFHFc2Jx9GdsUOVcOoFBehp9G8RF6QZRThCYg21Q8k3vWxCu7W++IWMtbOmqedvvNpstxfbkugnTkNqThaEtz4ZTU9XbaXiB14kV4sQSFx8HF5AlrRlKfS+InNdXNmy/uqt9MpZV9i9aW9WQW4j3XK9V08vshe82hD9L0I+FeVeyMLBnczg8= root@zfs-pod
`),
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
	newSnapshot := r.CachedData[migrationRequest.Name].StorageClass.Parameters["poolname"] + "/" + *vsContent.Status.SnapshotHandle
	r.CachedData[migrationRequest.Name].ConfigMap.Data["SNAPSHOT"] = newSnapshot

	// Apply the updated ConfigMap
	err = r.Update(ctx, r.CachedData[migrationRequest.Name].ConfigMap)
	if err != nil {
		return err
	}

	// Create the Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snapshot-sender-" + string(vs.UID),
			Namespace: "default",
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "zfs-container",
							Image: "thehamdiaz/zfs-ubuntu:v13.0",
							//	Command: []string{
							//		"/bin/sh",
							//		"-c",
							//		"sleep infinity",
							//	},
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

	// Check if the Job is created
	jobKey := types.NamespacedName{Name: job.Name, Namespace: job.Namespace}
	err = wait.PollImmediate(time.Second, time.Minute*5, func() (bool, error) {
		if err := r.Get(ctx, jobKey, job); err != nil {
			return false, err
		}

		if job.Status.StartTime != nil {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return err
	}

	// Wait for the Job to finish
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
	pod := &corev1.Pod{}

	if err := r.Get(ctx, types.NamespacedName{Namespace: migrationRequest.Namespace, Name: migrationRequest.Spec.PodName}, pod); err != nil {
		return err
	}

	// Set the pod's deletion timestamp to stop it
	gracePeriodSeconds := int64(30)
	deleteOptions := client.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}
	err := r.Delete(ctx, pod, &deleteOptions)
	if err != nil {
		return err
	}

	// Wait until the pod is deleted
	err = wait.PollImmediate(time.Second, time.Minute*5, func() (bool, error) {
		err := r.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, pod)
		if err != nil {
			if errors.IsNotFound(err) {
				// Pod is deleted
				return true, nil
			}
			return false, err
		}

		return false, nil
	})

	return err
}

func NewCachedResources() CachedResources {
	return CachedResources{
		Pod:                   &corev1.Pod{},
		StorageClass:          &storagev1.StorageClass{},
		PersistentVolume:      &corev1.PersistentVolume{},
		PersistentVolumeClaim: &corev1.PersistentVolumeClaim{},
		VolumeSnapshotClass:   &snapv1.VolumeSnapshotClass{},
		ConfigMap:             &corev1.ConfigMap{},
		Secret:                &corev1.Secret{},
		PreviousSnapshot:      &snapv1.VolumeSnapshot{},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.MigrationRequest{}).
		Complete(r)
}
