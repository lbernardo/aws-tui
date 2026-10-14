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

func newVolumesView(api *ec2.Client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Volumes",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Volume ID", Width: 21},
			{Title: "Size", Width: 8},
			{Title: "Type", Width: 6},
			{Title: "State", Width: 10},
			{Title: "AZ", Width: 11},
			{Title: "Attached to", Width: 19},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			p := ec2.NewDescribeVolumesPaginator(api, &ec2.DescribeVolumesInput{})
			for p.HasMorePages() {
				page, err := p.NextPage(ctx)
				if err != nil {
					return nil, err
				}
				for _, v := range page.Volumes {
					rows = append(rows, ui.Row{
						ID: aws.ToString(v.VolumeId),
						Cells: []string{
							ui.OrDash(ec2util.NameTag(v.Tags)),
							aws.ToString(v.VolumeId),
							fmt.Sprintf("%d GiB", aws.ToInt32(v.Size)),
							string(v.VolumeType),
							string(v.State),
							aws.ToString(v.AvailabilityZone),
							ui.OrDash(strings.Join(attachedInstances(v), ", ")),
						},
						Data: v,
					})
				}
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			v := r.Data.(types.Volume)
			return ui.Push(ui.NewDetail(ui.DetailConfig{
				Title: aws.ToString(v.VolumeId),
				Load:  func(context.Context) ([]ui.Field, error) { return volumeFields(v), nil },
				ID:    aws.ToString(v.VolumeId),
				Tags:  ec2util.IDTags(api, aws.ToString(v.VolumeId)),
			}))
		},
		Tags:      ec2util.RowTags(api),
		EmptyText: "No volumes in this region.",
	})
}

func attachedInstances(v types.Volume) []string {
	var ids []string
	for _, a := range v.Attachments {
		ids = append(ids, aws.ToString(a.InstanceId))
	}
	return ids
}

func volumeFields(v types.Volume) []ui.Field {
	var attachments []string
	for _, a := range v.Attachments {
		attachments = append(attachments, fmt.Sprintf("%s on %s (%s)", aws.ToString(a.Device), aws.ToString(a.InstanceId), a.State))
	}
	iops, throughput := "-", "-"
	if v.Iops != nil {
		iops = fmt.Sprint(aws.ToInt32(v.Iops))
	}
	if v.Throughput != nil {
		throughput = fmt.Sprintf("%d MiB/s", aws.ToInt32(v.Throughput))
	}
	fields := []ui.Field{
		{Name: "Volume ID", Value: aws.ToString(v.VolumeId)},
		{Name: "Name", Value: ui.OrDash(ec2util.NameTag(v.Tags))},
		{Name: "State", Value: string(v.State)},
		{Name: "Size", Value: fmt.Sprintf("%d GiB", aws.ToInt32(v.Size))},
		{Name: "Type", Value: string(v.VolumeType)},
		{Name: "IOPS", Value: iops},
		{Name: "Throughput", Value: throughput},
		{Name: "Encrypted", Value: fmt.Sprint(aws.ToBool(v.Encrypted))},
		{Name: "KMS key", Value: ui.OrDash(aws.ToString(v.KmsKeyId))},
		{Name: "Snapshot", Value: ui.OrDash(aws.ToString(v.SnapshotId))},
		{Name: "AZ", Value: aws.ToString(v.AvailabilityZone)},
		{Name: "Created", Value: ui.FormatTime(aws.ToTime(v.CreateTime))},
		{},
		{Name: "Attachments", Value: ui.OrDash(strings.Join(attachments, "\n"))},
	}
	return append(fields, ec2util.TagFields(v.Tags)...)
}
