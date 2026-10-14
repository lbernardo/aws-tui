// Package s3 exposes Amazon S3 buckets and objects in the TUI.
package s3

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/lbernardo/aws-tui/internal/service"
	"github.com/lbernardo/aws-tui/internal/ui"
)

func init() {
	service.Register(service.Service{
		ID:          "s3",
		Name:        "S3",
		Description: "Simple Storage Service: buckets and objects",
		New: func(ctx *service.Context) ui.View {
			return newBucketsView(newClient(ctx.AWS))
		},
	})
}

// client keeps one S3 client per region, since object operations must be
// sent to the bucket's own region.
type client struct {
	cfg      aws.Config
	mu       sync.Mutex
	regional map[string]*s3.Client
	regions  map[string]string // bucket -> region cache
}

func newClient(cfg aws.Config) *client {
	return &client{cfg: cfg, regional: map[string]*s3.Client{}, regions: map[string]string{}}
}

func (c *client) forRegion(region string) *s3.Client {
	if region == "" {
		region = c.cfg.Region
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl, ok := c.regional[region]; ok {
		return cl
	}
	cl := s3.NewFromConfig(c.cfg, func(o *s3.Options) { o.Region = region })
	c.regional[region] = cl
	return cl
}

func (c *client) home() *s3.Client { return c.forRegion("") }

// forBucket returns a client bound to the bucket's region.
func (c *client) forBucket(ctx context.Context, b bucket) (*s3.Client, string, error) {
	region, err := c.regionOf(ctx, b)
	if err != nil {
		return nil, "", err
	}
	return c.forRegion(region), region, nil
}

// regionOf returns the bucket region, resolving (and caching) it when
// ListBuckets did not return it.
func (c *client) regionOf(ctx context.Context, b bucket) (string, error) {
	if b.region != "" {
		return b.region, nil
	}
	c.mu.Lock()
	region, ok := c.regions[b.name]
	c.mu.Unlock()
	if ok {
		return region, nil
	}

	out, err := c.forRegion("us-east-1").GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(b.name)})
	if err != nil {
		return "", err
	}
	switch region = string(out.LocationConstraint); region {
	case "":
		region = "us-east-1"
	case "EU":
		region = "eu-west-1"
	}

	c.mu.Lock()
	c.regions[b.name] = region
	c.mu.Unlock()
	return region, nil
}
