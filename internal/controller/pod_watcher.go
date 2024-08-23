package controller

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	EgressAnnotationPrefix = "egress.pona.cybozu.com/"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	EgressName      string
	EgressNamespace string
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.handlePodDeletion(ctx, req.NamespacedName); err != nil {
				logger.Error(err, "failed to remove tunnel")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) shouldHandle(pod *corev1.Pod) bool {
	if pod.Spec.HostNetwork {
		// Egress feature is not available for Pods running in the host network.
		return false
	}

	return r.hasEgressAnnotation(pod)
}

// TODO
func (r *PodReconciler) handlePodDeletion(ctx context.Context, namespacedName types.NamespacedName) error {
	logger := log.FromContext(ctx)

	return nil
}

func (r *PodReconciler) hasEgressAnnotation(pod *corev1.Pod) bool {
	for k, name := range pod.Annotations {
		if !strings.HasPrefix(k, EgressAnnotationPrefix) {
			continue
		}

		if k[len(EgressAnnotationPrefix):] != r.EgressNamespace {
			continue
		}

		// shortcut for the most typical case
		if name == r.EgressName {
			return true
		}

		for _, n := range strings.Split(name, ",") {
			if n == r.EgressNamespace {
				return true
			}
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
