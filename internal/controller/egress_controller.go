package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	labelAppName       = "app.kubernetes.io/name"
	labelAppInstance   = "app.kubernetes.io/instance"
	labelAppComponent  = "app.kubernetes.io/component"
	labelsAppCreatedBy = "app.kubernetes.io/created-by"
)

const (
	egressDefaultCpuRequest  = "100m"
	egressDefaultMemRequest  = "200Mi"
	egressServiceAccountName = "egress"
	egressCRBName            = "egress"
	egressCRName             = "egress"
)

// TODO: Change this
const (
	EnvNode         = "PONA_NODE_NAME"
	EnvAddresses    = "PONA_POD_ADDRESSES"
	EnvPodNamespace = "PONA_POD_NAMESPACE"
	EnvPodName      = "PONA_POD_NAME"
	EnvEgressName   = "PONA_EGRESS_NAME"
)

// EgressReconciler reconciles a Egress object
type EgressReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Port         int32
	DefaultImage string
}

// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pona.cybozu.com,resources=egresses/finalizers,verbs=update

//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
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

	var eg ponav1beta1.Egress
	if err := r.Get(ctx, req.NamespacedName, &eg); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get Egress",
			"api_version", eg.APIVersion,
			"kind", eg.Kind,
			"name", req.Name,
			"namespace", req.Namespace,
		)
		return ctrl.Result{}, err
	}

	if !eg.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	defer func() {
		if err := r.updateStatus(ctx, &eg); err != nil {
			logger.Error(err, "/",
				"api_version", eg.APIVersion,
				"kind", eg.Kind,
				"name", eg.Name,
				"namespace", eg.Namespace)
		}
	}()

	if err := r.reconcileServiceAccount(ctx, &eg); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileCR(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileCRB(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, eg); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, &eg); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcilePDB(ctx, &eg); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *EgressReconciler) reconcileServiceAccount(ctx context.Context, eg *ponav1beta1.Egress) error {
	if eg == nil {
		return errors.New("eg is nil")
	}
	logger := log.FromContext(ctx)

	sa := &corev1.ServiceAccount{}
	name := egressServiceAccountName
	namespace := eg.Namespace

	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, sa); err != nil {
		if apierrors.IsNotFound(err) {
			sa.SetName(name)
			sa.SetNamespace(namespace)
			logger.Info("creating service account for egress",
				"name", name,
				"namespace", namespace,
			)
			return r.Create(ctx, sa)
		}
		return err
	}

	return nil
}

func (r *EgressReconciler) reconcileCR(ctx context.Context) error {
	logger := log.FromContext(ctx)

	cr := rbacv1.ClusterRole{}
	name := egressCRName

	if err := r.Get(ctx, client.ObjectKey{Name: name}, &cr); err != nil {
		if apierrors.IsNotFound(err) {
			cr.SetName(name)
			logger.Info("creating service account for egress",
				"name", name,
			)
			cr.Rules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list", "watch"},
				},
			}
			return r.Create(ctx, &cr)
		}
		return err
	}

	return nil
}

func (r *EgressReconciler) reconcileCRB(ctx context.Context) error {
	crb := rbacv1.ClusterRoleBinding{}

	if err := r.Get(ctx, client.ObjectKey{Name: egressCRBName}, &crb); err != nil {
		if apierrors.IsNotFound(err) {
			crb.SetName(egressCRBName)

			crb.RoleRef = rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     egressCRName,
			}

			if err := r.Create(ctx, &crb); err != nil {
				return fmt.Errorf("failed to create CRB resource: %w", err)
			}
		} else {
			return fmt.Errorf("unable to get CRB: %w", err)
		}
	}

	egresses := &ponav1beta1.EgressList{}
	if err := r.List(ctx, egresses); err != nil {
		return fmt.Errorf("unable to list Egress: %w", err)
	}
	namespaces := getNamespaces(egresses)

	subjects := make([]rbacv1.Subject, len(namespaces))
	for i, n := range namespaces {
		subjects[i] = rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      egressServiceAccountName,
			Namespace: n,
		}
	}

	newCrb := crb.DeepCopy()
	newCrb.Subjects = subjects
	patch := client.MergeFrom(&crb)

	if err := r.Patch(ctx, newCrb, patch); err != nil {
		return fmt.Errorf("failed to patch crb: %w", err)
	}

	return nil
}

func getNamespaces(egresses *ponav1beta1.EgressList) []string {
	nsMap := make(map[string]struct{})
	for _, eg := range egresses.Items {
		nsMap[eg.Namespace] = struct{}{}
	}
	namespaces := make([]string, 0, len(nsMap))
	for k := range nsMap {
		namespaces = append(namespaces, k)
	}
	sort.Strings(namespaces)
	return namespaces
}

func (r *EgressReconciler) reconcileDeployment(ctx context.Context, eg ponav1beta1.Egress) error {
	logger := log.FromContext(ctx)

	owner, err := controllerReference(eg, r.Scheme)
	if err != nil {
		return err
	}

	labels := appLabels(eg.Name)

	dep := appsv1apply.Deployment(eg.Name, eg.Namespace).
		WithLabels(labels).
		WithOwnerReferences(owner).
		WithSpec(appsv1apply.DeploymentSpec().
			WithReplicas(eg.Spec.Replicas).
			WithSelector(metav1apply.LabelSelector().WithMatchLabels(labels)).
			WithStrategy((*appsv1apply.DeploymentStrategyApplyConfiguration)(eg.Spec.Strategy)).
			WithTemplate(corev1apply.PodTemplateSpec().WithSpec((*corev1apply.PodSpecApplyConfiguration)(&eg.Spec.Template.Spec))),
		)

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		return err
	}
	patch := &unstructured.Unstructured{
		Object: obj,
	}

	var current appsv1.Deployment
	err = r.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, &current)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	currApplyConfig, err := appsv1apply.ExtractDeployment(&current, "egress-controller")
	if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(dep, currApplyConfig) {
		return nil
	}

	err = r.Patch(ctx, patch, client.Apply, &client.PatchOptions{
		FieldManager: "egress-controller",
		Force:        ptr.To(true),
	})

	if err != nil {
		logger.Error(err, "unable to create or update Deployment",
			"api_version", dep.APIVersion,
			"kind", dep.Kind,
			"name", dep.Name,
			"namespace", dep.Namespace,
		)
		return err
	}
	logger.Info("reconcile Deployment successfully",
		"api_version", dep.APIVersion,
		"kind", dep.Kind,
		"name", dep.Name,
		"namespace", dep.Namespace,
	)
	return nil
}

func (r *EgressReconciler) reconcileService(ctx context.Context, eg *ponav1beta1.Egress) error {
	logger := log.FromContext(ctx)

	svc := &corev1.Service{}
	svc.SetName(eg.Name)
	svc.SetNamespace(eg.Namespace)

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if svc.DeletionTimestamp != nil {
			return nil
		}

		if svc.Labels == nil {
			svc.Labels = make(map[string]string)
		}
		labels := appLabels(eg.Name)
		for k, v := range labels {
			svc.Labels[k] = v
		}

		// set immutable fields only for a new object
		if svc.CreationTimestamp.IsZero() {
			if err := ctrl.SetControllerReference(eg, svc, r.Scheme); err != nil {
				return err
			}
		}

		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{{
			Port:       r.Port,
			TargetPort: intstr.FromInt(int(r.Port)),
			Protocol:   corev1.ProtocolUDP,
		}}
		svc.Spec.SessionAffinity = eg.Spec.SessionAffinity
		if eg.Spec.SessionAffinityConfig != nil {
			sac := &corev1.SessionAffinityConfig{}
			eg.Spec.SessionAffinityConfig.DeepCopyInto(sac)
			svc.Spec.SessionAffinityConfig = sac
		}
		return nil
	})
	if err != nil {
		return err
	}

	if result != controllerutil.OperationResultNone {
		logger.Info("service is created or updated",
			"result", result,
			"api_version", svc.APIVersion,
			"kind", svc.Kind,
			"name", svc.Name,
			"namespace", svc.Namespace,
		)
	}
	return nil
}

func (r *EgressReconciler) reconcilePDB(ctx context.Context, eg *ponav1beta1.Egress) error {
	logger := log.FromContext(ctx)
	if eg.Spec.PodDisruptionBudget == nil {
		return nil
	}

	pdb := &policyv1.PodDisruptionBudget{}
	pdb.SetNamespace(eg.Namespace)
	pdb.SetName(eg.Name)

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, pdb, func() error {
		if pdb.DeletionTimestamp != nil {
			return nil
		}

		if pdb.Labels == nil {
			pdb.Labels = make(map[string]string)
		}
		for k, v := range appLabels(eg.Name) {
			pdb.Labels[k] = v
		}
		if pdb.CreationTimestamp.IsZero() {
			if err := ctrl.SetControllerReference(eg, pdb, r.Scheme); err != nil {
				return err
			}
		}
		if eg.Spec.PodDisruptionBudget.MinAvailable != nil {
			pdb.Spec.MinAvailable = eg.Spec.PodDisruptionBudget.MinAvailable
		}
		if eg.Spec.PodDisruptionBudget.MaxUnavailable != nil {
			pdb.Spec.MaxUnavailable = eg.Spec.PodDisruptionBudget.MaxUnavailable
		}
		pdb.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: appLabels(eg.Name),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update PDB: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		logger.Info("PDB is created or updated",
			"result", result,
			"api_version", pdb.APIVersion,
			"kind", pdb.Kind,
			"name", pdb.Name,
			"namespace", pdb.Namespace,
		)
	}

	return nil
}

func (r *EgressReconciler) updateStatus(ctx context.Context, eg *ponav1beta1.Egress) error {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: eg.Namespace, Name: eg.Name}, dep); err != nil {
		return fmt.Errorf("failed to get deployment for updateStatus: %w", err)
	}
	sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to convert labelSelector: %w", err)
	}
	selString := sel.String()

	if eg.Status.Selector == selString && eg.Status.Replicas == dep.Status.AvailableReplicas {
		// no change
		return nil
	}

	eg.Status.Selector = selString
	eg.Status.Replicas = dep.Status.AvailableReplicas
	return r.Status().Update(ctx, eg)
}

// addVolumes adds volumes required by pona
// TODO: change this
func (r *EgressReconciler) addVolumes(vols []corev1.Volume) []corev1.Volume {
	noRun := true
	for _, vol := range vols {
		if vol.Name == "run" {
			noRun = false
			break
		}
	}
	if noRun {
		vols = append(vols, corev1.Volume{
			Name: "run",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	vols = append(vols, corev1.Volume{
		Name: "modules",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/lib/modules",
			},
		},
	})
	return vols
}

func (r *EgressReconciler) addVolumeMounts(mounts []corev1.VolumeMount) []corev1.VolumeMount {
	noRun := true
	for _, m := range mounts {
		if m.Name == "run" {
			noRun = false
			break
		}
	}
	if noRun {
		mounts = append(mounts, corev1.VolumeMount{
			MountPath: "/run",
			Name:      "run",
			ReadOnly:  false,
		})
	}

	mounts = append(mounts, corev1.VolumeMount{
		MountPath: "/lib/modules",
		Name:      "modules",
		ReadOnly:  true,
	})

	return mounts
}

// SetupWithManager sets up the controller with the Manager.
func (r *EgressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ponav1beta1.Egress{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}

func appLabels(name string) map[string]string {
	return map[string]string{
		labelAppName:       "pona",
		labelAppInstance:   name,
		labelAppComponent:  "egress",
		labelsAppCreatedBy: "egress-controller",
	}
}

func controllerReference(eg ponav1beta1.Egress, scheme *runtime.Scheme) (*metav1apply.OwnerReferenceApplyConfiguration, error) {
	gvk, err := apiutil.GVKForObject(&eg, scheme)
	if err != nil {
		return nil, err
	}
	ref := metav1apply.OwnerReference().
		WithAPIVersion(gvk.GroupVersion().String()).
		WithKind(gvk.Kind).
		WithName(eg.Name).
		WithUID(eg.GetUID()).
		WithBlockOwnerDeletion(true).
		WithController(true)
	return ref, nil
}
