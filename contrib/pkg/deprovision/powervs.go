package deprovision

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	powervsutils "github.com/openshift/hive/contrib/pkg/utils/powervs"
	"github.com/openshift/installer/pkg/destroy/powervs"
	"github.com/openshift/installer/pkg/types"
	typespowervs "github.com/openshift/installer/pkg/types/powervs"
)

// powerVSDeprovisionOptions is the set of options to deprovision an PowerVS cluster
type powerVSDeprovisionOptions struct {
	baseDomain  string
	clusterName string
	logLevel    string
	infraID     string
	region      string
	zone        string
}

// NewDeprovisionPowerVSCommand is the entrypoint to create the IBM Cloud deprovision subcommand
func NewDeprovisionPowerVSCommand() *cobra.Command {
	opt := &powerVSDeprovisionOptions{}
	cmd := &cobra.Command{
		Use:   "powervs INFRAID --region=us-east --zone=us-east --base-domain=BASE_DOMAIN --cluster-name=CLUSTERNAME",
		Short: "Deprovision PowerVS assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("failed to complete options")
			}
			if err := opt.Validate(cmd); err != nil {
				log.WithError(err).Fatal("validation failed")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")

	// Required flags
	flags.StringVar(&opt.baseDomain, "base-domain", "", "cluster's base domain")
	flags.StringVar(&opt.clusterName, "cluster-name", "", "cluster's name")
	flags.StringVar(&opt.region, "region", "", "region in which to deprovision cluster")
	flags.StringVar(&opt.zone, "zone", "", "zone in which to deprovision cluster")

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *powerVSDeprovisionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	powervsutils.ConfigureCreds(client)

	/*
		// Create PowerVS Client
		powerVSAPIKey := os.Getenv(constants.PowerVSAPIKeyEnvVar)
		if powerVSAPIKey == "" {
			return fmt.Errorf("no %s env var set, cannot proceed", constants.PowerVSAPIKeyEnvVar)
		}
		powervsClient, err := powervsclient.NewClient(powerVSAPIKey)
		if err != nil {
			return errors.Wrap(err, "Unable to create PowerVS client")
		}*/

	return nil
}

// Validate ensures that option values make sense
func (o *powerVSDeprovisionOptions) Validate(cmd *cobra.Command) error {
	if o.region == "" {
		cmd.Usage()
		return fmt.Errorf("no --region provided, cannot proceed")
	}
	if o.zone == "" {
		cmd.Usage()
		return fmt.Errorf("no --zone provided, cannot proceed")
	}
	if o.baseDomain == "" {
		cmd.Usage()
		return fmt.Errorf("no --base-domain provided, cannot proceed")
	}
	if o.clusterName == "" {
		cmd.Usage()
		return fmt.Errorf("no --cluster-name provided, cannot proceed")
	}
	return nil
}

// Run executes the command
func (o *powerVSDeprovisionOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		ClusterName: o.clusterName,
		InfraID:     o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			PowerVS: &typespowervs.Metadata{
				BaseDomain: o.baseDomain,
				Region:     o.region,
				Zone:       o.zone,
			},
		},
	}

	destroyer, err := powervs.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}
