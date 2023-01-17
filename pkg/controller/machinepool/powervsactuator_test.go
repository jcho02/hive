package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	powervsprovider "github.com/openshift/cluster-api-provider-powervs/pkg/apis/powervsprovider/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1powervs "github.com/openshift/hive/apis/hive/v1/powervs"
	mockpowervs "github.com/openshift/hive/pkg/powervsclient/mock"
)

func TestPowerVSActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		mockPowerVSClient          func(*mockpowervs.MockAPI)
		expectedMachineSetReplicas map[string]int32
		expectedErr                bool
	}{
		{
			name:              "generate machinesets for default region zones",
			clusterDeployment: testPowerVSClusterDeployment(),
			pool:              testPowerVSPool(),
			mockPowerVSClient: func(client *mockpowervs.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{"test-region-1", "test-region-2", "test-region-3"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int32{
				generatePowerVSMachineSetName("worker", "1"): 1,
				generatePowerVSMachineSetName("worker", "2"): 1,
				generatePowerVSMachineSetName("worker", "3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified Zones",
			clusterDeployment: testPowerVSClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testPowerVSPool()
				p.Spec.Platform.PowerVS.Zones = []string{"test-region-A", "test-region-B", "test-region-C"}
				return p
			}(),
			expectedMachineSetReplicas: map[string]int32{
				generatePowerVSMachineSetName("worker", "A"): 1,
				generatePowerVSMachineSetName("worker", "B"): 1,
				generatePowerVSMachineSetName("worker", "C"): 1,
			},
		},
		{
			name:              "no zones returned for specified region",
			clusterDeployment: testPowerVSClusterDeployment(),
			pool:              testPowerVSPool(),
			mockPowerVSClient: func(client *mockpowervs.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{}, testRegion)
			},
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)

			powervsClient := mockpowervs.NewMockAPI(mockCtrl)

			if test.mockPowerVSClient != nil {
				test.mockPowerVSClient(powervsClient)
			}

			actuator := &PowerVSActuator{
				logger:        log.WithField("actuator", "powervsactuator_test"),
				powervsClient: powervsClient,
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test case")

				// Ensure the correct number of machinesets were generated
				if assert.Equal(t, len(test.expectedMachineSetReplicas), len(generatedMachineSets), "different number of machine sets generated than expected") {
					for _, ms := range generatedMachineSets {
						expReplicas, ok := test.expectedMachineSetReplicas[ms.Name]
						if assert.True(t, ok, fmt.Sprintf("machine set with name %s not expected", ms.Name)) {
							assert.Equal(t, expReplicas, *ms.Spec.Replicas, "unexpected number of replicas")
						}
					}
				}

				for _, ms := range generatedMachineSets {
				}
			}
		})
	}
}

func testPowerVSPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		PowerVS: &hivev1powervs.MachinePool{
			MemoryGiB:  32,
			ProcType:   "Shared",
			Processors: "0.5",
			SysType:    "s922",
		},
	}
	return p
}

func testPowerVSClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		PowerVS: &hivev1powervs.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "powervs-credentials",
			},
			Region: testRegion,
			Zone:   testZone,
		},
	}
	return cd
}

func generatePowerVSMachineSetName(leaseChar, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, leaseChar, zone)
}
