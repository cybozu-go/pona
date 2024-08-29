package constants

const (
	EgressAnnotationPrefix = "egress.pona.cybozu.com/"
)

// Keys in CNI_ARGS
const (
	PodNameKey      = "K8S_POD_NAME"
	PodNamespaceKey = "K8S_POD_NAMESPACE"
	PodContainerKey = "K8S_POD_INFRA_CONTAINER_ID"
)
