/*
Copyright 2025.

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

	mcpv1alpha1 "github.com/howlowck/mcp-server-k8s-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.lifeishao.com,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.lifeishao.com,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.lifeishao.com,resources=mcpservers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MCPServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the MCPServer instance
	var mcpServer mcpv1alpha1.MCPServer
	if err := r.Get(ctx, req.NamespacedName, &mcpServer); err != nil {
		log.Error(err, "unable to fetch MCPServer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Extract fields from the MCPServer spec
	name := mcpServer.Spec.Name
	transport := mcpServer.Spec.Transport
	command := mcpServer.Spec.Command
	args := mcpServer.Spec.Args
	image := mcpServer.Spec.Image
	env := mcpServer.Spec.Env

	log.Info("Reconciling MCPServer", "name", name, "transport", transport, "command", command, "args", args)

	mcpServerContainer := corev1.Container{
		Name:  name,
		Image: image,
		Env:   env,
		Stdin: true,
	}

	// create a deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: func() *int32 { i := int32(1); return &i }(),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					ShareProcessNamespace: &[]bool{true}[0],
					Containers: []corev1.Container{
						mcpServerContainer,
					},
				},
			},
		},
	}

	// set command to the string of name, truncate to 15 characters if too long
	targetCommand := name
	if len(targetCommand) > 15 {
		targetCommand = targetCommand[:15]
	}

	if transport == "stdio" {
		// add stdin and tty to mcp server container
		mcpServerContainer.Stdin = true
		mcpServerContainer.TTY = true
		stdioProxyContainer := corev1.Container{
			Name:  name + "stdio-proxy",
			Image: "howlowck/stdio-jsonrpc-proxy:0.4.0",
			Env: []corev1.EnvVar{
				{
					Name:  "TARGET_COMMAND",
					Value: targetCommand,
				},
			},
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, stdioProxyContainer)
	}

	// Set the MCPServer instance as the owner and controller
	if err := ctrl.SetControllerReference(&mcpServer, deployment, r.Scheme); err != nil {
		log.Error(err, "unable to set controller reference")
		return ctrl.Result{}, err
	}
	// Check if the deployment already exists
	found := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: req.Namespace}, found)
	if err != nil && client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch Deployment")
		return ctrl.Result{}, err
	}
	if err == nil {
		log.Info("Deployment already exists", "Deployment.Name", found.Name)
	}

	log.Info("Creating Deployment", "Deployment.Name", deployment.Name)
	err = r.Create(ctx, deployment)
	if err != nil {
		log.Error(err, "unable to create Deployment", "Deployment.Name", deployment.Name)
		return ctrl.Result{}, err
	}
	log.Info("Deployment created successfully", "Deployment.Name", deployment.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1alpha1.MCPServer{}).
		Named("mcpserver").
		Complete(r)
}
