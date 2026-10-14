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

type instance struct {
	id   string
	name string
}

func (i instance) label() string {
	if i.name == "" {
		return i.id
	}
	return fmt.Sprintf("%s (%s)", i.name, i.id)
}

func newInstancesView(api *ec2.Client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Instances",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Instance ID", Width: 19},
			{Title: "State", Width: 14},
			{Title: "Type", Width: 12},
			{Title: "AZ", Width: 11},
			{Title: "Private IP", Width: 15},
			{Title: "Public IP", Width: 15},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) { return listInstances(ctx, api) },
		OnSelect: func(r ui.Row) tea.Cmd {
			return ui.Push(newInstanceView(api, r.Data.(instance)))
		},
		Actions: instanceActions(api, func(r *ui.Row) (instance, bool) {
			if r == nil {
				return instance{}, false
			}
			return r.Data.(instance), true
		}),
		Tags:      ec2util.RowTags(api),
		EmptyText: "No instances in this region.",
	})
}

func listInstances(ctx context.Context, api *ec2.Client) ([]ui.Row, error) {
	var rows []ui.Row
	p := ec2.NewDescribeInstancesPaginator(api, &ec2.DescribeInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, res := range page.Reservations {
			for _, in := range res.Instances {
				inst := instance{id: aws.ToString(in.InstanceId), name: ec2util.NameTag(in.Tags)}
				az := ""
				if in.Placement != nil {
					az = aws.ToString(in.Placement.AvailabilityZone)
				}
				rows = append(rows, ui.Row{
					ID: inst.id,
					Cells: []string{
						ui.OrDash(inst.name),
						inst.id,
						stateLabel(in.State),
						string(in.InstanceType),
						ui.OrDash(az),
						ui.OrDash(aws.ToString(in.PrivateIpAddress)),
						ui.OrDash(aws.ToString(in.PublicIpAddress)),
					},
					Data: inst,
				})
			}
		}
	}
	return rows, nil
}

func stateLabel(s *types.InstanceState) string {
	if s == nil {
		return "-"
	}
	icon := map[types.InstanceStateName]string{
		types.InstanceStateNameRunning:      "●",
		types.InstanceStateNameStopped:      "○",
		types.InstanceStateNamePending:      "◐",
		types.InstanceStateNameStopping:     "◐",
		types.InstanceStateNameShuttingDown: "◐",
		types.InstanceStateNameTerminated:   "✕",
	}[s.Name]
	return strings.TrimSpace(icon + " " + string(s.Name))
}

// instanceActions are shared by the list (selected row) and the detail view.
func instanceActions(api *ec2.Client, target func(*ui.Row) (instance, bool)) []ui.Action {
	act := func(fn func(instance) tea.Cmd) func(*ui.Row) tea.Cmd {
		return func(r *ui.Row) tea.Cmd {
			if inst, ok := target(r); ok {
				return fn(inst)
			}
			return nil
		}
	}
	return []ui.Action{
		ui.NewAction("s", "start", act(func(i instance) tea.Cmd {
			return ui.Task("Starting "+i.label()+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{i.id}})
				return "Start requested for " + i.label(), err
			})
		})),
		ui.NewAction("S", "stop", act(func(i instance) tea.Cmd {
			return ui.Confirm("Stop "+i.label()+"?", ui.Task("Stopping "+i.label()+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{i.id}})
				return "Stop requested for " + i.label(), err
			}))
		})),
		ui.NewAction("b", "reboot", act(func(i instance) tea.Cmd {
			return ui.Confirm("Reboot "+i.label()+"?", ui.Task("Rebooting "+i.label()+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.RebootInstances(ctx, &ec2.RebootInstancesInput{InstanceIds: []string{i.id}})
				return "Reboot requested for " + i.label(), err
			}))
		})),
		ui.NewAction("x", "terminate", act(func(i instance) tea.Cmd {
			return ui.Confirm("TERMINATE "+i.label()+"? This cannot be undone", ui.Task("Terminating "+i.label()+"…", true, func(ctx context.Context) (string, error) {
				_, err := api.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{i.id}})
				return "Terminate requested for " + i.label(), err
			}))
		})),
	}
}

func newInstanceView(api *ec2.Client, inst instance) ui.View {
	title := inst.name
	if title == "" {
		title = inst.id
	}
	return ui.NewDetail(ui.DetailConfig{
		Title: title,
		Load: func(ctx context.Context) ([]ui.Field, error) {
			out, err := api.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{inst.id}})
			if err != nil {
				return nil, err
			}
			if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
				return nil, fmt.Errorf("instance %s not found", inst.id)
			}
			return instanceFields(out.Reservations[0].Instances[0]), nil
		},
		Actions: instanceActions(api, func(*ui.Row) (instance, bool) { return inst, true }),
		ID:      inst.id,
		Tags:    ec2util.IDTags(api, inst.id),
	})
}

func instanceFields(in types.Instance) []ui.Field {
	var az, profile string
	if in.Placement != nil {
		az = aws.ToString(in.Placement.AvailabilityZone)
	}
	if in.IamInstanceProfile != nil {
		profile = aws.ToString(in.IamInstanceProfile.Arn)
	}
	state := stateLabel(in.State)
	if r := aws.ToString(in.StateTransitionReason); r != "" {
		state += "  " + ui.MutedStyle.Render(r)
	}

	var sgs []string
	for _, g := range in.SecurityGroups {
		sgs = append(sgs, fmt.Sprintf("%s (%s)", aws.ToString(g.GroupName), aws.ToString(g.GroupId)))
	}
	var disks []string
	for _, b := range in.BlockDeviceMappings {
		if b.Ebs != nil {
			disks = append(disks, fmt.Sprintf("%s → %s", aws.ToString(b.DeviceName), aws.ToString(b.Ebs.VolumeId)))
		}
	}

	platform := aws.ToString(in.PlatformDetails)
	if platform == "" {
		platform = string(in.Platform)
	}

	fields := []ui.Field{
		{Name: "Instance ID", Value: aws.ToString(in.InstanceId)},
		{Name: "Name", Value: ui.OrDash(ec2util.NameTag(in.Tags))},
		{Name: "State", Value: state},
		{Name: "Type", Value: string(in.InstanceType)},
		{Name: "Platform", Value: ui.OrDash(platform)},
		{Name: "Architecture", Value: ui.OrDash(string(in.Architecture))},
		{Name: "AMI", Value: ui.OrDash(aws.ToString(in.ImageId))},
		{Name: "Launch time", Value: ui.FormatTime(aws.ToTime(in.LaunchTime))},
		{},
		{Name: "AZ", Value: ui.OrDash(az)},
		{Name: "VPC", Value: ui.OrDash(aws.ToString(in.VpcId))},
		{Name: "Subnet", Value: ui.OrDash(aws.ToString(in.SubnetId))},
		{Name: "Private IP", Value: ui.OrDash(aws.ToString(in.PrivateIpAddress))},
		{Name: "Private DNS", Value: ui.OrDash(aws.ToString(in.PrivateDnsName))},
		{Name: "Public IP", Value: ui.OrDash(aws.ToString(in.PublicIpAddress))},
		{Name: "Public DNS", Value: ui.OrDash(aws.ToString(in.PublicDnsName))},
		{Name: "Security groups", Value: ui.OrDash(strings.Join(sgs, "\n"))},
		{},
		{Name: "Key pair", Value: ui.OrDash(aws.ToString(in.KeyName))},
		{Name: "IAM profile", Value: ui.OrDash(profile)},
		{Name: "Volumes", Value: ui.OrDash(strings.Join(disks, "\n"))},
	}
	return append(fields, ec2util.TagFields(in.Tags)...)
}
