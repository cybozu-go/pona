package v1beta1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EgressSpec defines the desired state of Egress
type EgressSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Destinations is a list of IP networks in CIDR format.
	// +kubebuilder:validation:MinItems=1
	Destinations []string `json:"destinations"`

	// Replicas is the desired number of egress (SNAT) pods.
	// Defaults to 1.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas int32 `json:"replicas"`

	// Strategy describes how to replace existing pods with new ones.
	// Ref. https://pkg.go.dev/k8s.io/api/apps/v1?tab=doc#DeploymentStrategy
	// +optional
	Strategy *appsv1.DeploymentStrategy `json:"strategy,omitempty"`

	// Template is an optional template for egress pods.
	// A container named "egress" is special.  It is the main container of
	// egress pods and usually is not meant to be modified.
	// +optional
	Template *EgressPodTemplate `json:"template,omitempty"`

	// SessionAffinity is to specify the same field of Service for the Egress.
	// However, the default is changed from None to ClientIP.
	// Ref. https://pkg.go.dev/k8s.io/api/core/v1?tab=doc#ServiceSpec
	// +kubebuilder:validation:Enum=ClientIP;None
	// +kubebuilder:default=None
	// +optional
	SessionAffinity corev1.ServiceAffinity `json:"sessionAffinity,omitempty"`

	// SessionAffinityConfig is to specify the same field of Service for Egress.
	// Ref. https://pkg.go.dev/k8s.io/api/core/v1?tab=doc#ServiceSpec
	// +optional
	SessionAffinityConfig *corev1.SessionAffinityConfig `json:"sessionAffinityConfig,omitempty"`

	// PodDisruptionBudget is an optional PodDisruptionBudget for Egress NAT Gateways.
	// +optional
	PodDisruptionBudget *EgressPDBSpec `json:"podDisruptionBudget,omitempty"`
}

// EgressPodTemplate defines pod template for Egress
//
// This is almost the same as corev1.PodTemplate but is simplified to
// workaround JSON patch issues.
type EgressPodTemplate struct {
	// Metadata defines optional labels and annotations
	// +optional
	Metadata `json:"metadata,omitempty"`

	// Spec defines the pod template spec.
	// +optional
	Spec corev1.PodSpec `json:"spec,omitempty"`
}

// EgressPDB defines PDB for Egress
type EgressPDBSpec struct {
	// MinAvailable is the minimum number of pods that must be available at any given time.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// MaxUnavailable is the maximum number of pods that can be unavailable at any given time.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// Metadata defines a simplified version of ObjectMeta.
type Metadata struct {
	// Annotations are optional annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels are optional labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// EgressStatus defines the observed state of Egress
type EgressStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Replicas is copied from the underlying Deployment's status.replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Selector is a serialized label selector in string form.
	Selector string `json:"selector,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName={eg}
// +kubebuilder:subresource:scale:selectorpath=.status.selector,specpath=.spec.replicas,statuspath=.status.replicas

// Egress is the Schema for the egresses API
type Egress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EgressSpec   `json:"spec,omitempty"`
	Status EgressStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EgressList contains a list of Egress
type EgressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Egress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Egress{}, &EgressList{})
}
