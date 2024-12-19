/*
Copyright 2024.

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

package app

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "my-crd/api/app/v1"
)

// MyCrdReconciler reconciles a MyCrd object
type MyCrdReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=app.example.com,resources=mycrds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=app.example.com,resources=mycrds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=app.example.com,resources=mycrds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MyCrd object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *MyCrdReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("receive reconcile event", "name", req.String())

	logger.Info("get my crd object", "name", req.String())
	my_crd := &appv1.MyCrd{}
	if err := r.Client.Get(ctx, req.NamespacedName, my_crd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if my_crd.DeletionTimestamp != nil {
		logger.Info("my crd in deleting", "name", req.String())
		return ctrl.Result{}, nil
	}

	logger.Info("begin to sync my crd", "name", req.String())
	if err := r.syncMyCrd(ctx, my_crd); err != nil {
		logger.Error(err, "failed to sync my crd", "name", req.String())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MyCrdReconciler) syncMyCrd(ctx context.Context, obj *appv1.MyCrd) error {
	logger := log.FromContext(ctx)
	my_crd := obj.DeepCopy()

	name := types.NamespacedName{
		Namespace: my_crd.Namespace,
		Name:      my_crd.Name,
	}

	owner := []metav1.OwnerReference{
		{
			APIVersion:         my_crd.APIVersion,
			Kind:               my_crd.Kind,
			Name:               my_crd.Name,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
			UID:                my_crd.UID,
		},
	}

	labels := map[string]string{
		"my-crd-name": my_crd.Name,
	}

	meta := metav1.ObjectMeta{
		Name:            my_crd.Name,
		Namespace:       my_crd.Namespace,
		Labels:          labels,
		OwnerReferences: owner,
	}

	deploy := &v1.Deployment{}
	if err := r.Client.Get(ctx, name, deploy); err != nil {
		logger.Info("get deployment success")
		if !errors.IsNotFound(err) {
			return err
		}

		deployment := &v1.Deployment{
			ObjectMeta: meta,
			Spec:       r.getDeploymentSpec(my_crd, labels),
		}
		if err := r.Client.Create(ctx, deployment); err != nil {
			return nil
		}
		logger.Info("create Deployment success", "name", name.String())
	} else {
		want := r.getDeploymentSpec(my_crd, labels)
		get := r.getSpecFromDeployment(deploy)
		if !reflect.DeepEqual(want, get) {
			new := deploy.DeepCopy()
			new.Spec = want
			if err := r.Client.Update(ctx, new); err != nil {
				return err
			}
			logger.Info("update deployment success", "name", name.String())
		}
	}

	if *my_crd.Spec.Replicas == deploy.Status.ReadyReplicas {
		my_crd.Status.Code = "success"
	} else {
		my_crd.Status.Code = "failed"
	}

	r.Client.Status().Update(ctx, my_crd)

	return nil
}

func (r *MyCrdReconciler) getDeploymentSpec(crd *appv1.MyCrd, labels map[string]string) v1.DeploymentSpec {
	return v1.DeploymentSpec{
		Replicas: crd.Spec.Replicas,
		Selector: metav1.SetAsLabelSelector(labels),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "cy-crd",
						Image: crd.Spec.Image,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: crd.Spec.ContainerPort,
							},
						},
					},
				},
			},
		},
	}
}

func (r *MyCrdReconciler) getSpecFromDeployment(deployment *v1.Deployment) v1.DeploymentSpec {
	container := deployment.Spec.Template.Spec.Containers[0]
	return v1.DeploymentSpec{
		Replicas: deployment.Spec.Replicas,
		Selector: deployment.Spec.Selector,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: deployment.Spec.Template.Labels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  container.Name,
						Image: container.Image,
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: container.Ports[0].ContainerPort,
							},
						},
					},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyCrdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.MyCrd{}).
		Named("app-mycrd").
		Complete(r)
}
