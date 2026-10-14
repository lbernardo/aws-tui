// Package services wires every AWS service implementation into the registry.
// To add a service, blank-import its package here.
package services

import (
	_ "github.com/lbenardo/aws-tui/internal/services/ec2"
	_ "github.com/lbenardo/aws-tui/internal/services/s3"
	_ "github.com/lbenardo/aws-tui/internal/services/sqs"
	_ "github.com/lbenardo/aws-tui/internal/services/vpc"
)
