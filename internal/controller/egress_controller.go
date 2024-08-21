package controller

import (
	"context"
	"errors"
	"fmt"

	ponav1beta1 "github.com/cybozu-go/pona/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	rbacv1 "k8s.io/api/rbac/v1"
)

const (
	labelAppName      = "app.kubernetes.io/name"
	labelAppInstance  = "app.kubernetes.io/instance"
	labelAppComponent = "app.kubernetes.io/component"
)

const (
	egressImage              = "ghcr.io/cybozu-go/coil:2.7.2"
	egressDefaultCpuRequest  = "100m"
	egressDefaultMemRequest  = "200Mi"
	egressServiceAccountName = "egress"
	egressCRBName = "egress"
)

// TODO: Change this
const (
	EnvNode         = "COIL_NODE_NAME"
	EnvAddresses    = "COIL_POD_ADDRESSES"
	EnvPodNamespace = "COIL_POD_NAMESPACE"
	EnvPodName      = "COIL_POD_NAME"
	EnvEgressName   = "COIL_EGRESS_NAME"
)

// EgressReconciler reconciles a Egress object
type EgressReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Port int32
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
			logger.Error(err, "unable to update status",
				"api_version", eg.APIVersion,
				"kind", eg.Kind,
				"name", eg.Name,
				"namespace", eg.Namespace)
		}
	}()

	if err := r.reconcileServiceAccount(ctx, &eg); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, &eg); err != nil {
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
	if eg != nil {
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

func (r *EgressReconciler) reconcileCRB(ctx context.Context) error {
	crb := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, client.ObjectKey{Name: egressCRBName}, crb); err != nil {
		if apierrors.IsNotFound(err){
			// TODO create CRB
			return nil
		}
		return err
	}

}

func (r *EgressReconciler) reconcileDeployment(ctx context.Context, eg *ponav1beta1.Egress) error {
	logger := log.FromContext(ctx)

	dep := &appsv1.Deployment{}
	dep.SetName(eg.Name)
	dep.SetNamespace(eg.Namespace)

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, dep,
		func() error {
			if dep.DeletionTimestamp != nil {
				return nil
			}

			if dep.Labels == nil {
				dep.Labels = make(map[string]string)
			}

			labels := appLabels(eg.Name)
			for k, v := range labels {
				dep.Labels[k] = v
			}

			if dep.CreationTimestamp.IsZero() {
				if err := ctrl.SetControllerReference(eg, dep, r.Scheme); err != nil {
					return err
				}
				dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
			}

			if dep.Spec.Replicas == nil || *dep.Spec.Replicas != eg.Spec.Replicas {
				replicas := eg.Spec.Replicas
				dep.Spec.Replicas = &replicas
			}

			if eg.Spec.Strategy != nil {
				eg.Spec.Strategy.DeepCopyInto(&dep.Spec.Strategy)
			}

			r.reconcilePodTemplate(eg, dep)
			return nil
		})
	if err != nil {
		return fmt.Errorf("failed to create or update deployment: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		logger.Info("deployment is created or updated",
			"result", result,
			"api_version", dep.APIVersion,
			"kind", dep.Kind,
			"name", dep.Name,
			"namespace", dep.Namespace,
		)
	}

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

func (r *EgressReconciler) reconcilePodTemplate(eg *ponav1beta1.Egress, deploy *appsv1.Deployment) {
	target := &deploy.Spec.Template
	target.Labels = make(map[string]string)
	if target.Annotations == nil {
		target.Annotations = make(map[string]string)
	}

	desired := eg.Spec.Template
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

	for k, v := range appLabels(eg.Name) {
		target.Labels[k] = v
	}

	podSpec.ServiceAccountName = egressServiceAccountName
	podSpec.Volumes = r.addVolumes(podSpec.Volumes)

	var egressContainer *corev1.Container
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name != "egress" {
			continue
		}
		egressContainer = &(podSpec.Containers[i])
	}
	if egressContainer == nil {
		podSpec.Containers = append([]corev1.Container{{}}, podSpec.Containers...)
		egressContainer = &(podSpec.Containers[0])
	}

	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "egress" {
			continue
		}
		egressContainer = &(podSpec.Containers[i])
	}
	if egressContainer == nil {
		podSpec.Containers = append([]corev1.Container{{}}, podSpec.Containers...)
		egressContainer = &(podSpec.Containers[0])
	}
	egressContainer.Name = "egress"

	//TODO: Change image name and others from coil
	if egressContainer.Image == "" {
		egressContainer.Image = egressImage
	}
	if len(egressContainer.Command) == 0 {
		egressContainer.Command = []string{"coil-egress"}
	}
	if len(egressContainer.Args) == 0 {
		egressContainer.Args = []string{"--zap-stacktrace-level=panic"}
	}
	egressContainer.Env = append(egressContainer.Env,
		corev1.EnvVar{
			Name:  EnvPodNamespace,
			Value: eg.Namespace,
		},
		corev1.EnvVar{
			Name:  EnvEgressName,
			Value: eg.Name,
		},
		corev1.EnvVar{
			Name: EnvAddresses,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIPs",
				},
			},
		},
	)
	egressContainer.VolumeMounts = r.addVolumeMounts(egressContainer.VolumeMounts)
	egressContainer.SecurityContext = &corev1.SecurityContext{
		Privileged:             ptr.To(true),
		ReadOnlyRootFilesystem: ptr.To(true),
		Capabilities:           &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}},
	}
	if egressContainer.Resources.Requests == nil {
		egressContainer.Resources.Requests = make(corev1.ResourceList)
	}
	if _, ok := egressContainer.Resources.Requests[corev1.ResourceCPU]; !ok {
		egressContainer.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(egressDefaultCpuRequest)
	}
	if _, ok := egressContainer.Resources.Requests[corev1.ResourceMemory]; !ok {
		egressContainer.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(egressDefaultMemRequest)
	}
	egressContainer.Ports = []corev1.ContainerPort{
		{Name: "metrics", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
		{Name: "health", ContainerPort: 8081, Protocol: corev1.ProtocolTCP},
	}
	egressContainer.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path:   "/livez",
			Port:   intstr.FromString("livez"),
			Scheme: corev1.URISchemeHTTP,
		}},
	}
	egressContainer.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path:   "/readyz",
			Port:   intstr.FromString("health"),
			Scheme: corev1.URISchemeHTTP,
		}},
	}

	podSpec.DeepCopyInto(&target.Spec)
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

// addVolumes adds volumes required by coil
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
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}

func appLabels(name string) map[string]string {
	return map[string]string{
		labelAppName:      "pona",
		labelAppInstance:  name,
		labelAppComponent: "egress",
	}
}
