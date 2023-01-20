package clusterresource

import (
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	installertypes "github.com/openshift/installer/pkg/types"
	powervsinstallertypes "github.com/openshift/installer/pkg/types/powervs"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1powervs "github.com/openshift/hive/apis/hive/v1/powervs"
	"github.com/openshift/hive/pkg/constants"
)

var _ CloudBuilder = (*PowerVSBuilder)(nil)

// PowerVSBuilder encapsulates cluster artifact generation logic specific to PowerVS.
type PowerVSBuilder struct {
	// APIKey is the PowerVS api key
	APIKey string

	// Region specifies the PowerVS region where the cluster will be created
	Region string `json:"region"`

	// Zone specifies the PowerVS zone where the cluster will be created
	Zone string `json:"zone"`
}

func (p *PowerVSBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.CredsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			// This API KEY will be passed to the installer as constants.PowerVSAPIKeyEnvVar
			constants.PowerVSAPIKeySecretKey: p.APIKey,
		},
	}
}

func (p *PowerVSBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return nil
}

func (p *PowerVSBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		PowerVS: &hivev1powervs.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region: p.Region,
			Zone:   p.Zone,
		},
	}
}

func (p *PowerVSBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.PowerVS = &hivev1powervs.MachinePool{
		MemoryGiB:  32,
		Processors: intstr.FromString("0.5"),
		SysType:    "s922",
	}
}

func (p *PowerVSBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	ic.Platform = installertypes.Platform{
		PowerVS: &powervsinstallertypes.Platform{
			Region: p.Region,
			Zone:   p.Zone,
		},
	}
	// Used for both control plane and workers.
	mpp := &powervsinstallertypes.MachinePool{}
	ic.ControlPlane.Platform.PowerVS = mpp
	ic.Compute[0].Platform.PowerVS = mpp
}

func (p *PowerVSBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-powervs-creds", o.Name)
}
