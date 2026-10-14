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
	"github.com/lbenardo/aws-tui/internal/ui"
)

// ---- Subnets ---------------------------------------------------------------

func newSubnetsView(api *ec2.Client, vpcID string) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Subnets",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Subnet ID", Width: 24},
			{Title: "VPC", Width: 21},
			{Title: "CIDR", Width: 18},
			{Title: "AZ", Width: 11},
			{Title: "Free IPs", Width: 8},
			{Title: "Public", Width: 6},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeSubnetsPaginator(api, &ec2.DescribeSubnetsInput{Filters: vpcFilter("vpc-id", vpcID)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, s := range page.Subnets {
					rows = append(rows, ui.Row{
						ID: aws.ToString(s.SubnetId),
						Cells: []string{
							ui.OrDash(ec2util.NameTag(s.Tags)),
							aws.ToString(s.SubnetId),
							aws.ToString(s.VpcId),
							aws.ToString(s.CidrBlock),
							aws.ToString(s.AvailabilityZone),
							fmt.Sprint(aws.ToInt32(s.AvailableIpAddressCount)),
							yesNo(aws.ToBool(s.MapPublicIpOnLaunch)),
						},
						Data: s,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			s := r.Data.(types.Subnet)
			var ipv6 []string
			for _, a := range s.Ipv6CidrBlockAssociationSet {
				ipv6 = append(ipv6, aws.ToString(a.Ipv6CidrBlock))
			}
			fields := []ui.Field{
				{Name: "Subnet ID", Value: aws.ToString(s.SubnetId)},
				{Name: "Name", Value: ui.OrDash(ec2util.NameTag(s.Tags))},
				{Name: "State", Value: string(s.State)},
				{Name: "VPC", Value: aws.ToString(s.VpcId)},
				{Name: "IPv4 CIDR", Value: aws.ToString(s.CidrBlock)},
				{Name: "IPv6 CIDR", Value: ui.OrDash(strings.Join(ipv6, "\n"))},
				{Name: "AZ", Value: fmt.Sprintf("%s (%s)", aws.ToString(s.AvailabilityZone), aws.ToString(s.AvailabilityZoneId))},
				{Name: "Free IPs", Value: fmt.Sprint(aws.ToInt32(s.AvailableIpAddressCount))},
				{Name: "Auto-assign public IP", Value: yesNo(aws.ToBool(s.MapPublicIpOnLaunch))},
				{Name: "Default for AZ", Value: yesNo(aws.ToBool(s.DefaultForAz))},
				{Name: "ARN", Value: ui.OrDash(aws.ToString(s.SubnetArn))},
			}
			return detail(api, aws.ToString(s.SubnetId), label(ec2util.NameTag(s.Tags), aws.ToString(s.SubnetId)), append(fields, ec2util.TagFields(s.Tags)...))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No subnets found.",
	})
}

// ---- Route tables ----------------------------------------------------------

func newRouteTablesView(api *ec2.Client, vpcID string) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Route Tables",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Route table ID", Width: 21},
			{Title: "VPC", Width: 21},
			{Title: "Main", Width: 4},
			{Title: "Subnets", Width: 7},
			{Title: "Routes", Width: 6},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeRouteTablesPaginator(api, &ec2.DescribeRouteTablesInput{Filters: vpcFilter("vpc-id", vpcID)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, rt := range page.RouteTables {
					main, subnets := false, 0
					for _, a := range rt.Associations {
						if aws.ToBool(a.Main) {
							main = true
						}
						if a.SubnetId != nil {
							subnets++
						}
					}
					rows = append(rows, ui.Row{
						ID: aws.ToString(rt.RouteTableId),
						Cells: []string{
							ui.OrDash(ec2util.NameTag(rt.Tags)),
							aws.ToString(rt.RouteTableId),
							aws.ToString(rt.VpcId),
							yesNo(main),
							fmt.Sprint(subnets),
							fmt.Sprint(len(rt.Routes)),
						},
						Data: rt,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			rt := r.Data.(types.RouteTable)
			var assoc []string
			for _, a := range rt.Associations {
				switch {
				case aws.ToBool(a.Main):
					assoc = append(assoc, "main")
				case a.SubnetId != nil:
					assoc = append(assoc, aws.ToString(a.SubnetId))
				case a.GatewayId != nil:
					assoc = append(assoc, aws.ToString(a.GatewayId))
				}
			}
			fields := []ui.Field{
				{Name: "Route table ID", Value: aws.ToString(rt.RouteTableId)},
				{Name: "Name", Value: ui.OrDash(ec2util.NameTag(rt.Tags))},
				{Name: "VPC", Value: aws.ToString(rt.VpcId)},
				{Name: "Associations", Value: ui.OrDash(strings.Join(assoc, "\n"))},
				{},
				{Name: "Routes", Value: formatRoutes(rt.Routes)},
			}
			return detail(api, aws.ToString(rt.RouteTableId), label(ec2util.NameTag(rt.Tags), aws.ToString(rt.RouteTableId)), append(fields, ec2util.TagFields(rt.Tags)...))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No route tables found.",
	})
}

// formatRoutes renders one "destination → target" line per route.
func formatRoutes(routes []types.Route) string {
	var lines []string
	for _, r := range routes {
		dest := firstNonEmpty(aws.ToString(r.DestinationCidrBlock), aws.ToString(r.DestinationIpv6CidrBlock), aws.ToString(r.DestinationPrefixListId))
		target := firstNonEmpty(
			aws.ToString(r.GatewayId),
			aws.ToString(r.NatGatewayId),
			aws.ToString(r.TransitGatewayId),
			aws.ToString(r.VpcPeeringConnectionId),
			aws.ToString(r.InstanceId),
			aws.ToString(r.NetworkInterfaceId),
			aws.ToString(r.EgressOnlyInternetGatewayId),
			aws.ToString(r.CarrierGatewayId),
			aws.ToString(r.LocalGatewayId),
		)
		line := fmt.Sprintf("%-20s → %s", dest, target)
		if r.State == types.RouteStateBlackhole {
			line += "  " + ui.ErrorStyle.Render("blackhole")
		}
		lines = append(lines, line)
	}
	return ui.OrDash(strings.Join(lines, "\n"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return "-"
}

// ---- Internet gateways -----------------------------------------------------

func newInternetGatewaysView(api *ec2.Client, vpcID string) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Internet Gateways",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Gateway ID", Width: 21},
			{Title: "VPC", Width: 21},
			{Title: "State", Width: 10},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeInternetGatewaysPaginator(api, &ec2.DescribeInternetGatewaysInput{Filters: vpcFilter("attachment.vpc-id", vpcID)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, g := range page.InternetGateways {
					vpc, state := "-", "detached"
					if len(g.Attachments) > 0 {
						vpc, state = aws.ToString(g.Attachments[0].VpcId), string(g.Attachments[0].State)
					}
					rows = append(rows, ui.Row{
						ID:    aws.ToString(g.InternetGatewayId),
						Cells: []string{ui.OrDash(ec2util.NameTag(g.Tags)), aws.ToString(g.InternetGatewayId), vpc, state},
						Data:  g,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			g := r.Data.(types.InternetGateway)
			var att []string
			for _, a := range g.Attachments {
				att = append(att, fmt.Sprintf("%s (%s)", aws.ToString(a.VpcId), a.State))
			}
			fields := []ui.Field{
				{Name: "Gateway ID", Value: aws.ToString(g.InternetGatewayId)},
				{Name: "Name", Value: ui.OrDash(ec2util.NameTag(g.Tags))},
				{Name: "Attachments", Value: ui.OrDash(strings.Join(att, "\n"))},
				{Name: "Owner", Value: ui.OrDash(aws.ToString(g.OwnerId))},
			}
			return detail(api, aws.ToString(g.InternetGatewayId), label(ec2util.NameTag(g.Tags), aws.ToString(g.InternetGatewayId)), append(fields, ec2util.TagFields(g.Tags)...))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No internet gateways found.",
	})
}

// ---- NAT gateways ----------------------------------------------------------

func newNATGatewaysView(api *ec2.Client, vpcID string) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "NAT Gateways",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "NAT gateway ID", Width: 21},
			{Title: "VPC", Width: 21},
			{Title: "Subnet", Width: 24},
			{Title: "State", Width: 9},
			{Title: "Public IP", Width: 15},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeNatGatewaysPaginator(api, &ec2.DescribeNatGatewaysInput{Filter: vpcFilter("vpc-id", vpcID)})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, g := range page.NatGateways {
					publicIP := "-"
					if len(g.NatGatewayAddresses) > 0 {
						publicIP = ui.OrDash(aws.ToString(g.NatGatewayAddresses[0].PublicIp))
					}
					rows = append(rows, ui.Row{
						ID: aws.ToString(g.NatGatewayId),
						Cells: []string{
							ui.OrDash(ec2util.NameTag(g.Tags)),
							aws.ToString(g.NatGatewayId),
							aws.ToString(g.VpcId),
							aws.ToString(g.SubnetId),
							string(g.State),
							publicIP,
						},
						Data: g,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			g := r.Data.(types.NatGateway)
			var addrs []string
			for _, a := range g.NatGatewayAddresses {
				addrs = append(addrs, fmt.Sprintf("%s → %s (%s)",
					ui.OrDash(aws.ToString(a.PublicIp)), aws.ToString(a.PrivateIp), ui.OrDash(aws.ToString(a.AllocationId))))
			}
			fields := []ui.Field{
				{Name: "NAT gateway ID", Value: aws.ToString(g.NatGatewayId)},
				{Name: "Name", Value: ui.OrDash(ec2util.NameTag(g.Tags))},
				{Name: "State", Value: string(g.State)},
				{Name: "Connectivity", Value: string(g.ConnectivityType)},
				{Name: "VPC", Value: aws.ToString(g.VpcId)},
				{Name: "Subnet", Value: aws.ToString(g.SubnetId)},
				{Name: "Addresses", Value: ui.OrDash(strings.Join(addrs, "\n"))},
				{Name: "Created", Value: ui.FormatTime(aws.ToTime(g.CreateTime))},
			}
			if msg := aws.ToString(g.FailureMessage); msg != "" {
				fields = append(fields, ui.Field{Name: "Failure", Value: ui.ErrorStyle.Render(msg)})
			}
			return detail(api, aws.ToString(g.NatGatewayId), label(ec2util.NameTag(g.Tags), aws.ToString(g.NatGatewayId)), append(fields, ec2util.TagFields(g.Tags)...))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No NAT gateways found.",
	})
}
