// Package ec2 exposes Amazon EC2 instances, security groups and volumes.
package ec2

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/lbenardo/aws-tui/internal/service"
	"github.com/lbenardo/aws-tui/internal/ui"
)

func init() {
	service.Register(service.Service{
		ID:          "ec2",
		Name:        "EC2",
		Description: "Elastic Compute Cloud: instances, security groups and volumes",
		New: func(ctx *service.Context) ui.View {
			return newMenuView(ec2.NewFromConfig(ctx.AWS))
		},
	})
}

func newMenuView(api *ec2.Client) ui.View {
	return ui.NewMenu("EC2", []ui.MenuItem{
		{Name: "Instances", Description: "Virtual machines", Open: func() ui.View { return newInstancesView(api) }},
		{Name: "Security Groups", Description: "Virtual firewalls for instances", Open: func() ui.View { return newSecurityGroupsView(api) }},
		{Name: "Volumes", Description: "EBS block storage", Open: func() ui.View { return newVolumesView(api) }},
	})
}
