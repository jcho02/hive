package awsprivatelink

import (
	"context"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveutils "github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	operatorutils "github.com/openshift/hive/pkg/operator/hive"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type disableOptions struct {
	dynamicClient client.Client
}

func NewDisableAWSPrivateLinkCommand() *cobra.Command {
	opt := &disableOptions{}

	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable AWS PrivateLink",
		Long: `Disable AWS PrivateLink:
1) Remove Secret(s) with AWS hub account credentials created when calling "hiveutil awsprivatelink enable ..."
2) Empty HiveConfig.spec.awsPrivateLink`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			if err := opt.Validate(cmd, args); err != nil {
				return
			}
			if err := opt.Run(cmd, args); err != nil {
				return
			}
		},
	}
	return cmd
}

func (o *disableOptions) Complete(cmd *cobra.Command, args []string) error {
	// Get controller-runtime dynamic client
	dynamicClient, err := hiveutils.GetClient()
	if err != nil {
		log.WithError(err).Fatal("Failed to create controller-runtime client")
	}
	o.dynamicClient = dynamicClient

	return nil
}

func (o *disableOptions) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *disableOptions) Run(cmd *cobra.Command, args []string) error {
	// Get HiveConfig
	hiveConfig := &hivev1.HiveConfig{}
	if err := o.dynamicClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to get HiveConfig/hive")
	}
	if hiveConfig.Spec.AWSPrivateLink == nil {
		log.Warn("AWS PrivateLink is already disabled in HiveConfig")
	}
	// It is assumed that no cloud resources for networking present
	// if and only if there is either no endpoint VPC or no associated VPC.
	if hiveConfig.Spec.AWSPrivateLink != nil &&
		len(hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory) > 0 &&
		len(hiveConfig.Spec.AWSPrivateLink.AssociatedVPCs) > 0 {
		log.Fatal("HiveConfig has at least 1 associated VPC and 1 endpoint VPC specified. " +
			"Please either remove all associated VPCs or all endpoint VPCs from it. " +
			"Please remember to delete relevant cloud resources for networking " +
			"(between the associated VPCs and endpoint VPCs) as you remove the VPCs." +
			"You can remove the endpoint VPCs (and relevant cloud resources for networking) " +
			"one by one via `hiveutil awsprivatelink endpointvpc remove ...`.")
	}

	// Delete Hub account secret(s) if present
	hiveNS := operatorutils.GetHiveNamespace(hiveConfig)
	hubAcctSecrets := &corev1.SecretList{}
	if err := o.dynamicClient.List(
		context.Background(),
		hubAcctSecrets,
		client.MatchingFields{"metadata.name": awsutils.PrivateLinkHubAcctCredsName},
		client.MatchingLabels{awsutils.PrivateLinkHubAcctCredsLabel: "true"},
		client.InNamespace(hiveNS),
	); err != nil {
		log.WithError(err).Error("Failed to list Hub account credentials Secrets")
	}

	for _, hubAcctSecret := range hubAcctSecrets.Items {
		if err := o.dynamicClient.Delete(context.Background(), &hubAcctSecret); err != nil {
			log.WithError(err).Errorf("Failed to delete Hub account credentials Secret %v", hubAcctSecret.Name)
		} else {
			log.Infof("Hub account credentials Secret %v deleted", hubAcctSecret.Name)
		}
	}

	// Empty HiveConfig.spec.awsPrivateLink
	hiveConfig.Spec.AWSPrivateLink = nil
	if err := o.dynamicClient.Update(context.Background(), hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to update HiveConfig")
	}
	log.Info("HiveConfig updated")

	return nil
}
