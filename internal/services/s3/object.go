package s3

import (
	"context"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbenardo/aws-tui/internal/ui"
)

const presignTTL = time.Hour

// newObjectView shows the metadata of a single object.
func newObjectView(c *client, b bucket, key string) ui.View {
	return ui.NewDetail(ui.DetailConfig{
		Title: path.Base(key),
		Load: func(ctx context.Context) ([]ui.Field, error) {
			api, region, err := c.forBucket(ctx, b)
			if err != nil {
				return nil, err
			}
			out, err := api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)})
			if err != nil {
				return nil, err
			}
			size := aws.ToInt64(out.ContentLength)

			fields := []ui.Field{
				{Name: "Bucket", Value: b.name},
				{Name: "Key", Value: key},
				{Name: "Region", Value: region},
				{Name: "S3 URI", Value: fmt.Sprintf("s3://%s/%s", b.name, key)},
				{},
				{Name: "Size", Value: fmt.Sprintf("%s (%d bytes)", ui.HumanBytes(size), size)},
				{Name: "Last modified", Value: ui.FormatTime(aws.ToTime(out.LastModified))},
				{Name: "Content type", Value: ui.OrDash(aws.ToString(out.ContentType))},
				{Name: "ETag", Value: ui.OrDash(aws.ToString(out.ETag))},
				{Name: "Storage class", Value: ui.OrDash(string(out.StorageClass))},
				{Name: "Encryption", Value: ui.OrDash(string(out.ServerSideEncryption))},
				{Name: "Version ID", Value: ui.OrDash(aws.ToString(out.VersionId))},
			}

			if len(out.Metadata) > 0 {
				fields = append(fields, ui.Field{})
				keys := make([]string, 0, len(out.Metadata))
				for k := range out.Metadata {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fields = append(fields, ui.Field{Name: "x-amz-meta-" + k, Value: out.Metadata[k]})
				}
			}

			req, err := s3.NewPresignClient(api).PresignGetObject(ctx,
				&s3.GetObjectInput{Bucket: aws.String(b.name), Key: aws.String(key)},
				s3.WithPresignExpires(presignTTL))
			if err == nil {
				fields = append(fields, ui.Field{}, ui.Field{Name: "Presigned URL (1h)", Value: req.URL})
			}
			return fields, nil
		},
		ID:   fmt.Sprintf("s3://%s/%s", b.name, key),
		Tags: func(ctx context.Context) (map[string]string, error) { return objectTags(ctx, c, b, key) },
		Actions: []ui.Action{
			ui.NewAction("d", "download", func(*ui.Row) tea.Cmd { return promptDownload(c, b, key) }),
		},
	})
}
