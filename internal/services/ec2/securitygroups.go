package ec2

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

func newSecurityGroupsView(api *ec2.Client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Security Groups",
		Columns: []ui.Column{
			{Title: "Group ID", Width: 20},
			{Title: "Name", Width: 28},
			{Title: "VPC", Width: 21},
			{Title: "In", Width: 4},
			{Title: "Out", Width: 4},
			{Title: "Description"},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeSecurityGroupsPaginator(api, &ec2.DescribeSecurityGroupsInput{})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, g := range page.SecurityGroups {
					rows = append(rows, ui.Row{
						ID: aws.ToString(g.GroupId),
						Cells: []string{
							aws.ToString(g.GroupId),
							aws.ToString(g.GroupName),
							ui.OrDash(aws.ToString(g.VpcId)),
							fmt.Sprint(len(g.IpPermissions)),
							fmt.Sprint(len(g.IpPermissionsEgress)),
							aws.ToString(g.Description),
						},
						Data: g,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			g := r.Data.(types.SecurityGroup)
			return ui.Push(ui.NewDetail(ui.DetailConfig{
				Title: aws.ToString(g.GroupName),
				Load:  func(context.Context) ([]ui.Field, error) { return securityGroupFields(g), nil },
				ID:    aws.ToString(g.GroupId),
				Tags:  ec2util.IDTags(api, aws.ToString(g.GroupId)),
			}))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No security groups in this region.",
	})
}

func securityGroupFields(g types.SecurityGroup) []ui.Field {
	fields := []ui.Field{
		{Name: "Group ID", Value: aws.ToString(g.GroupId)},
		{Name: "Name", Value: aws.ToString(g.GroupName)},
		{Name: "Description", Value: ui.OrDash(aws.ToString(g.Description))},
		{Name: "VPC", Value: ui.OrDash(aws.ToString(g.VpcId))},
		{Name: "Owner", Value: ui.OrDash(aws.ToString(g.OwnerId))},
		{},
		{Name: "Inbound rules", Value: formatRules(g.IpPermissions)},
		{},
		{Name: "Outbound rules", Value: formatRules(g.IpPermissionsEgress)},
	}
	return append(fields, ec2util.TagFields(g.Tags)...)
}

// formatRules renders one line per (protocol, port range, source).
func formatRules(perms []types.IpPermission) string {
	var lines []string
	for _, p := range perms {
		proto, ports := aws.ToString(p.IpProtocol), "all"
		if proto == "-1" {
			proto = "all"
		} else if p.FromPort != nil {
			from, to := aws.ToInt32(p.FromPort), aws.ToInt32(p.ToPort)
			ports = fmt.Sprint(from)
			if from != to {
				ports = fmt.Sprintf("%d-%d", from, to)
			}
		}
		rule := func(source, desc string) {
			line := fmt.Sprintf("%-5s %-11s %s", proto, ports, source)
			if desc != "" {
				line += "  " + ui.MutedStyle.Render(desc)
			}
			lines = append(lines, line)
		}
		for _, r := range p.IpRanges {
			rule(aws.ToString(r.CidrIp), aws.ToString(r.Description))
		}
		for _, r := range p.Ipv6Ranges {
			rule(aws.ToString(r.CidrIpv6), aws.ToString(r.Description))
		}
		for _, r := range p.UserIdGroupPairs {
			rule(aws.ToString(r.GroupId), aws.ToString(r.Description))
		}
		for _, r := range p.PrefixListIds {
			rule(aws.ToString(r.PrefixListId), aws.ToString(r.Description))
		}
	}
	if len(lines) == 0 {
		return "-"
	}
	return strings.Join(lines, "\n")
}
