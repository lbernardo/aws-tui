// Package vpc exposes Amazon VPC networking resources: VPCs, subnets, route
// tables, internet gateways and NAT gateways.
package vpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbenardo/aws-tui/internal/ec2util"
	"github.com/lbenardo/aws-tui/internal/service"
	"github.com/lbenardo/aws-tui/internal/ui"
)

func init() {
	service.Register(service.Service{
		ID:          "vpc",
		Name:        "VPC",
		Description: "Virtual Private Cloud: VPCs, subnets, route tables and gateways",
		New: func(ctx *service.Context) ui.View {
			return newMenuView(ec2.NewFromConfig(ctx.AWS), "VPC", "")
		},
	})
}

// newMenuView lists the networking resources. When vpcID is set every list
// is scoped to that VPC.
func newMenuView(api *ec2.Client, title, vpcID string) ui.View {
	var items []ui.MenuItem
	if vpcID == "" {
		items = append(items, ui.MenuItem{Name: "VPCs", Description: "Isolated virtual networks", Open: func() ui.View { return newVPCsView(api) }})
	} else {
		items = append(items, ui.MenuItem{Name: "Details", Description: "Attributes of this VPC", Open: func() ui.View { return newVPCDetailView(api, vpcID) }})
	}
	items = append(items,
		ui.MenuItem{Name: "Subnets", Description: "IP ranges inside the VPC", Open: func() ui.View { return newSubnetsView(api, vpcID) }},
		ui.MenuItem{Name: "Route Tables", Description: "Routing rules for subnets", Open: func() ui.View { return newRouteTablesView(api, vpcID) }},
		ui.MenuItem{Name: "Internet Gateways", Description: "Internet access for the VPC", Open: func() ui.View { return newInternetGatewaysView(api, vpcID) }},
		ui.MenuItem{Name: "NAT Gateways", Description: "Outbound internet for private subnets", Open: func() ui.View { return newNATGatewaysView(api, vpcID) }},
	)
	return ui.NewMenu(title, items)
}

// vpcFilter returns an EC2 filter scoping a listing to a VPC, if any.
func vpcFilter(name, vpcID string) []types.Filter {
	if vpcID == "" {
		return nil
	}
	return []types.Filter{{Name: aws.String(name), Values: []string{vpcID}}}
}

// detail opens a detail view for fields already loaded by a listing.
func detail(api *ec2.Client, id, title string, fields []ui.Field) tea.Cmd {
	return ui.Push(ui.NewDetail(ui.DetailConfig{
		Title: title,
		Load:  func(context.Context) ([]ui.Field, error) { return fields, nil },
		ID:    id,
		Tags:  ec2util.IDTags(api, id),
	}))
}

// label returns "name (id)" or just the id.
func label(name, id string) string {
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// ---- VPCs ------------------------------------------------------------------

func newVPCsView(api *ec2.Client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "VPCs",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "VPC ID", Width: 21},
			{Title: "IPv4 CIDR", Width: 18},
			{Title: "State", Width: 9},
			{Title: "Default", Width: 7},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeVpcsPaginator(api, &ec2.DescribeVpcsInput{})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, v := range page.Vpcs {
					rows = append(rows, ui.Row{
						ID: aws.ToString(v.VpcId),
						Cells: []string{
							ui.OrDash(ec2util.NameTag(v.Tags)),
							aws.ToString(v.VpcId),
							aws.ToString(v.CidrBlock),
							string(v.State),
							yesNo(aws.ToBool(v.IsDefault)),
						},
						Data: v,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			v := r.Data.(types.Vpc)
			id := aws.ToString(v.VpcId)
			title := ec2util.NameTag(v.Tags)
			if title == "" {
				title = id
			}
			return ui.Push(newMenuView(api, title, id))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No VPCs in this region.",
	})
}

func newVPCDetailView(api *ec2.Client, vpcID string) ui.View {
	return ui.NewDetail(ui.DetailConfig{
		Title: "Details",
		ID:    vpcID,
		Tags:  ec2util.IDTags(api, vpcID),
		Load: func(ctx context.Context) ([]ui.Field, error) {
			out, err := api.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{vpcID}})
			if err != nil {
				return nil, err
			}
			if len(out.Vpcs) == 0 {
				return nil, fmt.Errorf("VPC %s not found", vpcID)
			}
			v := out.Vpcs[0]
			var cidrs []string
			for _, a := range v.CidrBlockAssociationSet {
				cidrs = append(cidrs, fmt.Sprintf("%s (%s)", aws.ToString(a.CidrBlock), stateOf(a.CidrBlockState)))
			}
			for _, a := range v.Ipv6CidrBlockAssociationSet {
				cidrs = append(cidrs, aws.ToString(a.Ipv6CidrBlock))
			}
			fields := []ui.Field{
				{Name: "VPC ID", Value: vpcID},
				{Name: "Name", Value: ui.OrDash(ec2util.NameTag(v.Tags))},
				{Name: "State", Value: string(v.State)},
				{Name: "Default", Value: yesNo(aws.ToBool(v.IsDefault))},
				{Name: "CIDR blocks", Value: ui.OrDash(strings.Join(cidrs, "\n"))},
				{Name: "Tenancy", Value: string(v.InstanceTenancy)},
				{Name: "DHCP options", Value: ui.OrDash(aws.ToString(v.DhcpOptionsId))},
				{Name: "Owner", Value: ui.OrDash(aws.ToString(v.OwnerId))},
			}
			return append(fields, ec2util.TagFields(v.Tags)...), nil
		},
	})
}

func stateOf(s *types.VpcCidrBlockState) string {
	if s == nil {
		return "-"
	}
	return string(s.State)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
