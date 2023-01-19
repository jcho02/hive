package manageddns

import (
	"encoding/json"
	"os"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// ReadManagedDomainsFile reads the managed domains from the file pointed to
// by the ManagedDomainsFileEnvVar environment variable.
func ReadManagedDomainsFile() ([]hivev1.ManageDNSConfig, error) {
	managedDomainsFile := os.Getenv(constants.ManagedDomainsFileEnvVar)
	if len(managedDomainsFile) == 0 {
		return nil, nil
	}

	domains := []hivev1.ManageDNSConfig{}

	fileBytes, err := os.ReadFile(managedDomainsFile)
	if err != nil {
		return domains, err
	}
	if err := json.Unmarshal(fileBytes, &domains); err != nil {
		return domains, err
	}

	return domains, nil
}
