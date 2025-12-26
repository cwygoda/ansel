package publish

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// AWSClients holds initialized AWS service clients.
type AWSClients struct {
	Route53        *route53.Client
	CloudFormation *cloudformation.Client
	CloudFront     *cloudfront.Client
	S3             *s3.Client
	Config         aws.Config
}

// NewAWSClients creates AWS clients using the standard credential chain.
// If profile is non-empty, uses that named profile.
// If region is non-empty, overrides the default region.
func NewAWSClients(ctx context.Context, profile, region string) (*AWSClients, error) {
	var opts []func(*config.LoadOptions) error

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// All resources are created in us-east-1:
	// - Route53 is a global service but uses us-east-1 for API calls
	// - CloudFormation runs in us-east-1 for ACM certificates (required for CloudFront)
	// - S3 bucket is created by CloudFormation in us-east-1
	usEast1Cfg := cfg.Copy()
	usEast1Cfg.Region = "us-east-1"

	return &AWSClients{
		Route53:        route53.NewFromConfig(usEast1Cfg),
		CloudFormation: cloudformation.NewFromConfig(cfg),
		CloudFront:     cloudfront.NewFromConfig(usEast1Cfg),
		S3:             s3.NewFromConfig(usEast1Cfg),
		Config:         cfg,
	}, nil
}

// CloudFormationUsEast1 returns a CloudFormation client for us-east-1.
// Required for ACM certificates used with CloudFront.
func (c *AWSClients) CloudFormationUsEast1() *cloudformation.Client {
	cfg := c.Config.Copy()
	cfg.Region = "us-east-1"
	return cloudformation.NewFromConfig(cfg)
}
