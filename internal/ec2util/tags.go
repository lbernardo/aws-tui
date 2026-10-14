// Package ec2util holds helpers shared by services built on the EC2 API
// (EC2, VPC...).
package ec2util

import (
	"context"

	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/lbenardo/aws-tui/internal/ui"
)

// NameTag returns the value of the "Name" tag.
func NameTag(tags []types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == "Name" {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

// TagFields renders tags (except Name) sorted by key, preceded by a blank
// separator line.
func TagFields(tags []types.Tag) []ui.Field {
	var fields []ui.Field
	for _, t := range tags {
		if k := aws.ToString(t.Key); k != "Name" {
			fields = append(fields, ui.Field{Name: "tag:" + k, Value: aws.ToString(t.Value)})
		}
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	if len(fields) > 0 {
		fields = append([]ui.Field{{}}, fields...)
	}
	return fields
}

// LoadTags fetches the current tags of any EC2-API resource by ID
// (instances, volumes, security groups, VPCs, subnets, gateways...).
func LoadTags(ctx context.Context, api *ec2.Client, resourceID string) (map[string]string, error) {
	tags := map[string]string{}
	p := ec2.NewDescribeTagsPaginator(api, &ec2.DescribeTagsInput{
		Filters: []types.Filter{{Name: aws.String("resource-id"), Values: []string{resourceID}}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range page.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// RowTags returns a ui.TagsFunc that loads tags for the row's ID.
func RowTags(api *ec2.Client) ui.TagsFunc {
	return func(ctx context.Context, row ui.Row) (map[string]string, error) {
		return LoadTags(ctx, api, row.ID)
	}
}

// IDTags returns a tags loader for a detail view.
func IDTags(api *ec2.Client, resourceID string) func(context.Context) (map[string]string, error) {
	return func(ctx context.Context) (map[string]string, error) { return LoadTags(ctx, api, resourceID) }
}
