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

package controller

import (
	"context"
	"fmt"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	net "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	jobicov1 "github.com/andrescosta/jobicok8s/api/v1"
)

// ListenerReconciler reconciles a Listener object
type ListenerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=jobico.coeux.dev,resources=listeners,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=jobico.coeux.dev,resources=listeners/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=jobico.coeux.dev,resources=listeners/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Listener object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *ListenerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var listener jobicov1.Listener
	if err := r.Get(ctx, req.NamespacedName, &listener); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	fmt.Printf("creating %s", listener.Name)
	for _, e := range listener.Spec.Events {
		// Deployment
		repls := int32(1)
		class := "nginx"
		dm := int32(420)
		pt := net.PathType("Prefix")
		d := apps.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "listener",
				Namespace: "default",
			},
			Spec: apps.DeploymentSpec{
				Replicas: &repls,
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{"app": "listener"},
				},
				Template: core.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{"app": "listener"},
					},
					Spec: core.PodSpec{
						RestartPolicy: core.RestartPolicyAlways,
						Volumes: []core.Volume{
							{
								Name: "schema",
								VolumeSource: core.VolumeSource{
									ConfigMap: &core.ConfigMapVolumeSource{
										DefaultMode: &dm,
										LocalObjectReference: core.LocalObjectReference{
											Name: "schema-" + e.Name,
										},
									},
								},
							},
						},
						Containers: []core.Container{
							{
								Name:            "listener",
								Image:           "listener:v1",
								ImagePullPolicy: core.PullNever,
								Ports: []core.ContainerPort{
									{
										ContainerPort: 8080,
									},
								},
								Env: []core.EnvVar{
									{
										Name:  "event",
										Value: e.Name,
									},
								},
								VolumeMounts: []core.VolumeMount{
									{
										Name:      "schema",
										MountPath: "/etc/listener",
										// SubPath:   "schema.json",
									},
								},
							},
						},
					},
				},
			},
		}
		if err := ctrl.SetControllerReference(&listener, &d, r.Scheme); err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		if err := r.Create(ctx, &d); err != nil {
			fmt.Printf("error: %v\n", err)
		}
		// Service
		s := core.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:      "listener",
				Namespace: "default",
				Labels:    map[string]string{"app": "listener"},
			},
			Spec: core.ServiceSpec{
				Selector: map[string]string{"app": "listener"},
				Ports:    []core.ServicePort{{Port: 8080, TargetPort: intstr.FromInt(8080)}},
				Type:     core.ServiceTypeClusterIP,
			},
		}
		if err := ctrl.SetControllerReference(&listener, &s, r.Scheme); err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		if err := r.Create(ctx, &s); err != nil {
			fmt.Printf("error: %v\n", err)
		}

		// Ingress
		i := net.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Name:        "listener-ingress-http",
				Namespace:   "default",
				Annotations: map[string]string{"nginx.ingress.kubernetes.io/ssl-redirect": "true"},
			},
			Spec: net.IngressSpec{
				IngressClassName: &class,
				Rules: []net.IngressRule{
					{
						Host: "listener",
						IngressRuleValue: net.IngressRuleValue{
							HTTP: &net.HTTPIngressRuleValue{
								Paths: []net.HTTPIngressPath{
									{
										Path:     "/",
										PathType: &pt,
										Backend: net.IngressBackend{
											Service: &net.IngressServiceBackend{
												Name: "listener",
												Port: net.ServiceBackendPort{
													Number: 8080,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		if err := ctrl.SetControllerReference(&listener, &i, r.Scheme); err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		if err := r.Create(ctx, &i); err != nil {
			fmt.Printf("error: %v\n", err)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ListenerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jobicov1.Listener{}).
		Complete(r)
}
