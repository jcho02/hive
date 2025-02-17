package endpointvpc

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveutils "github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	"github.com/openshift/hive/pkg/awsclient"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type endpointVPCRemoveOptions struct {
	hiveConfig        hivev1.HiveConfig
	associatedVpcs    []hivev1.AWSAssociatedVPC
	endpointVpcId     string
	endpointVpcRegion string
	endpointVpcIdx    int
	endpointSubnetIds []string

	dynamicClient      client.Client
	endpointVpcClients awsclient.Client
	awsClientsByRegion map[string]awsclient.Client
}

func NewEndpointVPCRemoveCommand() *cobra.Command {
	opt := &endpointVPCRemoveOptions{}

	cmd := &cobra.Command{
		Use:   "remove vpc-id",
		Short: "Tear down the networking elements between an endpoint VPC and each associated VPC",
		Long: `Tear down the networking elements between an endpoint VPC and each associated VPC 
specified in HiveConfig.spec.awsPrivateLink.associatedVPCs:
1) Delete the VPC peering connection between the endpoint VPC and the associated VPC
2) Delete the route (to the peered VPC) in relevant route tables of the associated VPC and the endpoint VPC 
3) Remove the inbound rule from relevant SGs in the associated VPC and the endpoint VPC that permits traffic between them
4) Remove the endpoint VPC from HiveConfig.spec.awsPrivateLink.endpointVPCInventory`,
		Args: cobra.ExactArgs(1),
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

func (o *endpointVPCRemoveOptions) Complete(cmd *cobra.Command, args []string) error {
	o.endpointVpcId = args[0]

	// Get controller-runtime dynamic client
	dynamicClient, err := hiveutils.GetClient()
	if err != nil {
		log.WithError(err).Fatal("Failed to create controller-runtime client")
	}
	o.dynamicClient = dynamicClient

	// Get HiveConfig
	if err = o.dynamicClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, &o.hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to get HiveConfig/hive")
	}
	if o.hiveConfig.Spec.AWSPrivateLink == nil {
		log.Fatal(`AWS PrivateLink is not enabled in HiveConfig. Please call "hiveutil awsprivatelink enable" first`)
	}
	o.associatedVpcs = o.hiveConfig.Spec.AWSPrivateLink.AssociatedVPCs
	if len(o.associatedVpcs) == 0 {
		log.Warn("HiveConfig/hive does not specify any associated VPC. " +
			"The endpoint VPC passed in as argument will still be removed from HiveConfig, " +
			"and yet there will be no deletion of cloud resources.")
	}

	// Get endpoint VPC and AWS clients for it
	endpointVpcIdx, ok := awsutils.FindVpcInInventory(o.endpointVpcId, o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory)
	if !ok {
		log.Fatalf("Endpoint VPC %v not found in HiveConfig.spec.awsPrivateLink.endpointVPCInventory. "+
			"Please call `hiveutil privatelink endpointvpc add ...` to add it first", o.endpointVpcId)
	}
	o.endpointVpcIdx = endpointVpcIdx
	o.endpointVpcRegion = o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[endpointVpcIdx].AWSPrivateLinkVPC.Region
	for _, subnet := range o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[endpointVpcIdx].Subnets {
		o.endpointSubnetIds = append(o.endpointSubnetIds, subnet.SubnetID)
	}

	// Get AWS clients by region
	regions := sets.New(o.endpointVpcRegion)
	for _, associatedVpc := range o.associatedVpcs {
		regions.Insert(associatedVpc.AWSPrivateLinkVPC.Region)
	}
	o.awsClientsByRegion, err = awsutils.GetAWSClientsByRegion(regions)
	if err != nil {
		log.WithError(err).Fatal("Failed to get AWS clients")
	}
	// A shortcut to AWS clients of the endpoint VPC
	o.endpointVpcClients = o.awsClientsByRegion[o.endpointVpcRegion]

	return nil
}

func (o *endpointVPCRemoveOptions) Validate(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *endpointVPCRemoveOptions) Run(cmd *cobra.Command, args []string) error {
	// Get default SG of the endpoint VPC
	endpointVPCDefaultSG, err := awsutils.GetDefaultSGOfVpc(o.endpointVpcClients, aws.String(o.endpointVpcId))
	if err != nil {
		log.WithError(err).Fatal("Failed to get default SG of the endpoint VPC")
	}
	log.Debugf("Found default SG %v of the endpoint VPC", endpointVPCDefaultSG)

	// Remove the networking elements between the endpoint VPC and each associated VPC
	for _, associatedVpc := range o.associatedVpcs {
		associatedVpcRegion := associatedVpc.AWSPrivateLinkVPC.Region
		associatedVpcClients := o.awsClientsByRegion[associatedVpcRegion]
		associatedVpcId := associatedVpc.AWSPrivateLinkVPC.VPCID
		log.Infof("Removing networking elements between associated VPC %v and endpoint VPC %v", associatedVpcId, o.endpointVpcId)

		associatedVpcCIDR, err := awsutils.GetCIDRFromVpcId(associatedVpcClients, aws.String(associatedVpcId))
		if err != nil {
			log.Fatal("Failed to get CIDR of associated VPC")
		}
		log.Debugf("Found associated VPC CIDR = %v", associatedVpcCIDR)
		endpointVpcCIDR, err := awsutils.GetCIDRFromVpcId(o.endpointVpcClients, aws.String(o.endpointVpcId))
		if err != nil {
			log.Fatal("Failed to get CIDR of endpoint VPC")
		}
		log.Debugf("Found endpoint VPC CIDR = %v", endpointVpcCIDR)

		// Delete VPC peering connection
		if err = deleteVpcPeeringConnection(
			associatedVpcClients,
			aws.String(associatedVpcId),
			aws.String(o.endpointVpcId),
		); err != nil {
			log.WithError(err).Fatal("Failed to delete VPC peering connection")
		}

		// Update route tables
		log.Info("Deleting route from private route tables of the associated VPC")
		if err = deleteRouteFromRouteTables(
			associatedVpcClients,
			aws.String(associatedVpcId),
			aws.String(endpointVpcCIDR),
			&ec2.Filter{Name: aws.String("tag:Name"), Values: []*string{aws.String("*private*")}},
		); err != nil {
			log.WithError(err).Fatal("Failed to delete route from private route tables of the associated VPC")
		}

		log.Info("Deleting route from route tables of the endpoint subnets")
		if err = deleteRouteFromRouteTables(
			o.endpointVpcClients,
			aws.String(o.endpointVpcId),
			aws.String(associatedVpcCIDR),
			&ec2.Filter{Name: aws.String("association.subnet-id"), Values: aws.StringSlice(o.endpointSubnetIds)},
		); err != nil {
			log.WithError(err).Fatal("Failed to delete route from route tables of the endpoint subnets")
		}

		// Update SGs
		associatedVpcWorkerSG, err := awsutils.GetWorkerSGFromVpcId(associatedVpcClients, aws.String(associatedVpcId))
		if err != nil {
			log.WithError(err).Fatal("Failed to get worker SG of the associated Hive cluster")
		}
		log.Debugf("Found worker SG %v of the associated Hive cluster", associatedVpcWorkerSG)

		switch {

		// Associated VPC & endpoint VPC in the same region => revoke ingress from SG of the peer
		case associatedVpcRegion == o.endpointVpcRegion:
			log.Info("Revoking access from the endpoint VPC's default SG to the associated VPC's worker SG")
			if _, err = awsutils.RevokeAllIngressFromSG(
				associatedVpcClients,
				aws.String(associatedVpcWorkerSG),
				aws.String(endpointVPCDefaultSG),
			); err != nil {
				// Proceed if ingress not found, fail otherwise
				switch aerr, ok := err.(awserr.Error); {
				case ok && aerr.Code() == "InvalidPermission.NotFound":
					log.Warnf("Access from the endpoint VPC's default SG to the associated VPC's worker SG is not enabled")
				default:
					log.WithError(err).Fatal("Failed to revoke access from the endpoint VPC's default SG to the associated VPC's worker SG")
				}
			}

			log.Info("Revoking access from the associated VPC's worker SG to the endpoint VPC's default SG")
			if _, err = awsutils.RevokeAllIngressFromSG(
				o.endpointVpcClients,
				aws.String(endpointVPCDefaultSG),
				aws.String(associatedVpcWorkerSG),
			); err != nil {
				// Proceed if ingress not found, fail otherwise
				switch aerr, ok := err.(awserr.Error); {
				case ok && aerr.Code() == "InvalidPermission.NotFound":
					log.Warnf("Access from the associated VPC's worker SG to the endpoint VPC's default SG is not enabled")
				default:
					log.WithError(err).Fatal("Failed to revoke access from the associated VPC's worker SG to the endpoint VPC's default SG")
				}
			}

		// Associated VPC & endpoint VPC in different regions => revoke ingress from CIDR of the peer
		default:
			log.Info("Revoking access from the endpoint VPC's CIDR block to the associated VPC's worker SG")
			if _, err = awsutils.RevokeAllIngressFromCIDR(
				associatedVpcClients,
				aws.String(associatedVpcWorkerSG),
				aws.String(endpointVpcCIDR),
			); err != nil {
				// Proceed if ingress not found, fail otherwise
				switch aerr, ok := err.(awserr.Error); {
				case ok && aerr.Code() == "InvalidPermission.NotFound":
					log.Warnf("Access from the endpoint VPC's CIDR block to the associated VPC's worker SG is not enabled")
				default:
					log.WithError(err).Fatal("Failed to revoke access from the endpoint VPC's CIDR block to the associated VPC's worker SG")
				}
			}

			log.Info("Revoking access from the associated VPC's CIDR block to the endpoint VPC's default SG")
			if _, err = awsutils.RevokeAllIngressFromCIDR(
				o.endpointVpcClients,
				aws.String(endpointVPCDefaultSG),
				aws.String(associatedVpcCIDR),
			); err != nil {
				// Proceed if ingress not found, fail otherwise
				switch aerr, ok := err.(awserr.Error); {
				case ok && aerr.Code() == "InvalidPermission.NotFound":
					log.Warnf("Access from the associated VPC's CIDR block to the endpoint VPC's default SG is not enabled")
				default:
					log.WithError(err).Fatal("Failed to revoke access from the associated VPC's CIDR block to the endpoint VPC's default SG")
				}
			}
		}
	}

	// Update HiveConfig
	o.removeEndpointVpcFromHiveConfig()

	return nil
}

func (o *endpointVPCRemoveOptions) removeEndpointVpcFromHiveConfig() {
	log.Infof("Removing endpoint VPC %v from HiveConfig", o.endpointVpcId)

	// Remove endpoint VPC from HiveConfig if necessary
	o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[o.endpointVpcIdx] =
		o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[len(o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory)-1]
	o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory =
		o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[:len(o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory)-1]
	if err := o.dynamicClient.Update(context.Background(), &o.hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to update HiveConfig")
	}
}

func deleteVpcPeeringConnection(awsClients awsclient.Client, VpcId1, VpcId2 *string) error {
	log.Info("Deleting VPC peering connection between the associated VPC and the endpoint VPC")

	describeVpcPeeringConnectionsOutput, err := awsClients.DescribeVpcPeeringConnections(&ec2.DescribeVpcPeeringConnectionsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("requester-vpc-info.vpc-id"),
				Values: []*string{VpcId1, VpcId2},
			},
			{
				Name:   aws.String("accepter-vpc-info.vpc-id"),
				Values: []*string{VpcId1, VpcId2},
			},
			// Only one peering connection can be active at any given time between a pair of VPCs
			{
				Name:   aws.String("status-code"),
				Values: []*string{aws.String("active")},
			},
		},
	})
	if err != nil {
		return err
	}
	if len(describeVpcPeeringConnectionsOutput.VpcPeeringConnections) == 0 {
		log.Warn("No VPC peering connection found between the associated VPC and the endpoint VPC")
		return nil
	}

	VpcPeeringConnectionId := describeVpcPeeringConnectionsOutput.VpcPeeringConnections[0].VpcPeeringConnectionId
	if _, err = awsClients.DeleteVpcPeeringConnection(&ec2.DeleteVpcPeeringConnectionInput{
		VpcPeeringConnectionId: VpcPeeringConnectionId,
	}); err != nil {
		return err
	}
	log.Debugf("The deletion of VPC peering connection %v has been initiated", *VpcPeeringConnectionId)

	if err = awsClients.WaitUntilVpcPeeringConnectionDeleted(&ec2.DescribeVpcPeeringConnectionsInput{
		VpcPeeringConnectionIds: []*string{VpcPeeringConnectionId},
	}); err != nil {
		return err
	}
	log.Debugf("VPC peering connection %v deleted", *VpcPeeringConnectionId)

	return nil
}

func deleteRouteFromRouteTables(
	vpcClients awsclient.Client,
	vpcId, peerCIDR *string,
	additionalFiltersForRouteTables ...*ec2.Filter,
) error {
	filters := append([]*ec2.Filter{
		{
			Name:   aws.String("vpc-id"),
			Values: []*string{vpcId},
		},
	}, additionalFiltersForRouteTables...)

	return vpcClients.DescribeRouteTablesPages(
		&ec2.DescribeRouteTablesInput{
			Filters: filters,
		},
		func(page *ec2.DescribeRouteTablesOutput, lastPage bool) bool {
			for _, routeTable := range page.RouteTables {
				_, err := vpcClients.DeleteRoute(&ec2.DeleteRouteInput{
					RouteTableId:         routeTable.RouteTableId,
					DestinationCidrBlock: peerCIDR,
				})
				if err != nil {
					// Proceed if route not found, fail otherwise
					switch aerr, ok := err.(awserr.Error); {
					case ok && aerr.Code() == "InvalidRoute.NotFound":
						log.Warnf("Route not found in route table %v", *routeTable.RouteTableId)
					default:
						log.WithError(err).Fatalf("Failed to delete route from route table %v", *routeTable.RouteTableId)
					}
				} else {
					log.Debugf("Route deleted from route table %v", *routeTable.RouteTableId)
				}
			}

			return !lastPage
		},
	)
}
