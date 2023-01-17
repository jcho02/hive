package powervs

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global configuration that all machinesets use.
type Platform struct {
	// CredentialsSecretRef refers to a secret that contains IBM Cloud account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
	// Region specifies the PowerVS region where the cluster will be created
	Region string `json:"region"`
	// Zone specifies the PowerVS zone where the cluster will be created
	Zone string `json:"zone"`
}
