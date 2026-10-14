package s3

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/ui"
)

type bucket struct {
	name    string
	region  string
	created time.Time
}

func newBucketsView(c *client) ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Buckets",
		Columns: []ui.Column{
			{Title: "Name"},
			{Title: "Region", Width: 16},
			{Title: "Created", Width: 19},
		},
		Load:     func(ctx context.Context) ([]ui.Row, error) { return listBuckets(ctx, c) },
		OnSelect: func(r ui.Row) tea.Cmd { return ui.Push(newObjectsView(c, r.Data.(bucket), "")) },
		Actions: []ui.Action{
			ui.NewAction("c", "create", func(*ui.Row) tea.Cmd { return promptCreateBucket(c) }),
			ui.NewAction("x", "delete", func(r *ui.Row) tea.Cmd {
				if r == nil {
					return nil
				}
				return confirmDeleteBucket(c, r.Data.(bucket))
			}),
		},
		Tags: func(ctx context.Context, r ui.Row) (map[string]string, error) {
			return bucketTags(ctx, c, r.Data.(bucket))
		},
		EmptyText: "No buckets in this account. Press c to create one.",
	})
}

func listBuckets(ctx context.Context, c *client) ([]ui.Row, error) {
	var rows []ui.Row
	p := s3.NewListBucketsPaginator(c.home(), &s3.ListBucketsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, b := range page.Buckets {
			bk := bucket{
				name:    aws.ToString(b.Name),
				region:  aws.ToString(b.BucketRegion),
				created: aws.ToTime(b.CreationDate),
			}
			region := bk.region
			if region == "" {
				region = "-"
			}
			rows = append(rows, ui.Row{ID: bk.name, Cells: []string{bk.name, region, ui.FormatTime(bk.created)}, Data: bk})
		}
	}
	return rows, nil
}

func promptCreateBucket(c *client) tea.Cmd {
	return ui.Prompt("New bucket name:", "", func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		region := c.cfg.Region
		return ui.Task(fmt.Sprintf("Creating bucket %s in %s…", name, region), true, func(ctx context.Context) (string, error) {
			in := &s3.CreateBucketInput{Bucket: aws.String(name)}
			// us-east-1 is the only region that must not send a location constraint.
			if region != "us-east-1" {
				in.CreateBucketConfiguration = &types.CreateBucketConfiguration{
					LocationConstraint: types.BucketLocationConstraint(region),
				}
			}
			if _, err := c.forRegion(region).CreateBucket(ctx, in); err != nil {
				return "", err
			}
			return fmt.Sprintf("Bucket %s created", name), nil
		})
	})
}

func confirmDeleteBucket(c *client, b bucket) tea.Cmd {
	return ui.Confirm(fmt.Sprintf("Delete bucket %s? (it must be empty)", b.name),
		ui.Task("Deleting bucket "+b.name+"…", true, func(ctx context.Context) (string, error) {
			api, _, err := c.forBucket(ctx, b)
			if err != nil {
				return "", err
			}
			if _, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(b.name)}); err != nil {
				if strings.Contains(err.Error(), "BucketNotEmpty") {
					return "", fmt.Errorf("bucket %s is not empty", b.name)
				}
				return "", err
			}
			return fmt.Sprintf("Bucket %s deleted", b.name), nil
		}))
}

func bucketTags(ctx context.Context, c *client, b bucket) (map[string]string, error) {
	api, _, err := c.forBucket(ctx, b)
	if err != nil {
		return nil, err
	}
	out, err := api.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(b.name)})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchTagSet" {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return tagMap(out.TagSet), nil
}

func tagMap(tags []types.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}
