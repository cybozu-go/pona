package controller

import (
	"context"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	labelAppName      = "app.kubernetes.io/name"
	labelAppInstance  = "app.kubernetes.io/instance"
	labelAppComponent = "app.kubernetes.io/component"
)

// EgressReconciler reconciles a Egress object
type EgressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses/finalizers,verbs=update

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Egress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *EgressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var egress ponav1beta1.Egress
	err := r.Get(ctx, req.NamespacedName, &egress)
	if errors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "unable to get Egress",
			"name", req.Name,
			"namespace", req.Namespace,
		)
		return ctrl.Result{}, err
	}

	if !egress.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if err = r.reconcileDeployment(ctx, egress); err != nil {
		result, err2 := r.updateStatus(ctx, egress)
		logger.Error(err2, "unable to update status")
		return result, err
	}

	if err = r.reconcileService(ctx, egress); err != nil {
		result, err2 := r.updateStatus(ctx, egress)
		logger.Error(err2, "unable to update status")
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *EgressReconciler) reconcileDeployment(ctx context.Context, egress ponav1beta1.Egress) error {
	logger := log.FromContext(ctx)

	dep := &appsv1.Deployment{}
	dep.SetNamespace(egress.Namespace)
	dep.SetName(egress.Name)

	return nil
}

func (r *EgressReconciler) reconcilePodTemplate(egress *ponav1beta1.Egress, deploy *appsv1.Deployment) error {
	target := &deploy.Spec.Template
	target.Labels = make(map[string]string)
	if target.Annotations == nil {
		target.Annotations = make(map[string]string)
	}

	desired := egress.Spec.Template
	podSpec := &corev1.PodSpec{}
	if desired != nil {
		podSpec = desired.Spec.DeepCopy()
		for k, v := range desired.Annotations {
			target.Annotations[k] = v
		}
		for k, v := range desired.Labels {
			target.Labels[k] = v
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EgressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ponav1beta1.Egress{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func appLabels(name string) map[string]string {
	return map[string]string{
		labelAppName:      "pona",
		labelAppInstance:  name,
		labelAppComponent: "egress",
	}
}