package machinepool

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installpowervs "github.com/openshift/installer/pkg/asset/machines/powervs"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypespowervs "github.com/openshift/installer/pkg/types/powervs"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/powervsclient"
)

// PowerVSActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type PowerVSActuator struct {
	logger        log.FieldLogger
	powervsClient powervsclient.API
}

var _ Actuator = &PowerVSActuator{}

// NewPowerVSActuator is the constructor for building an PowerVSActuator
func NewPowerVSActuator(powervsCreds *corev1.Secret, scheme *runtime.Scheme, logger log.FieldLogger) (*PowerVSActuator, error) {
	powervsClient, err := powervsclient.NewClientFromSecret(powervsCreds)
	if err != nil {
		logger.WithError(err).Warn("failed to create PowerVS client with creds in clusterDeployment's secret")
		return nil, err
	}
	actuator := &PowerVSActuator{
		logger:        logger,
		powervsClient: powervsClient,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *PowerVSActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.PowerVS == nil {
		return nil, false, errors.New("ClusterDeployment is not for PowerVS")
	}
	if pool.Spec.Platform.PowerVS == nil {
		return nil, false, errors.New("MachinePool is not for PowerVS")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.PowerVS = &installertypespowervs.MachinePool{
		MemoryGiB:  pool.Spec.Platform.PowerVS.MemoryGiB,
		ProcType:   pool.Spec.Platform.PowerVS.ProcType,
		Processors: pool.Spec.Platform.PowerVS.Processors,
		SysType:    pool.Spec.Platform.PowerVS.SysType,
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			PowerVS: &installertypespowervs.Platform{
				Region: cd.Spec.Platform.PowerVS.Region,
				Zone:   cd.Spec.Platform.PowerVS.Zone,
			},
		},
	}

	installerMachineSets, err := installpowervs.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}
