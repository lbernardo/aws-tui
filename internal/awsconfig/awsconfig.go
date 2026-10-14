// Package awsconfig loads the AWS SDK configuration from the usual sources
// (env vars, shared config/credentials files, SSO, IMDS...).
package awsconfig

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// DefaultRegion is used when no region is configured anywhere.
const DefaultRegion = "us-east-1"

// Load returns the AWS config for the given profile and region. Empty values
// fall back to the SDK defaults (AWS_PROFILE, AWS_REGION, ~/.aws/config).
func Load(ctx context.Context, profile, region string) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}
	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}
	return cfg, nil
}

// ProfileName returns the profile that is effectively in use.
func ProfileName(profile string) string {
	if profile != "" {
		return profile
	}
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return p
	}
	return "default"
}
