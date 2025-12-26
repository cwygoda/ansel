package publish

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

// StackParams holds parameters for stack creation/update.
type StackParams struct {
	StackName    string
	Subdomain    string
	DomainName   string
	HostedZoneID string
}

// StackOutputs holds the outputs from a deployed stack.
type StackOutputs struct {
	BucketName         string
	DistributionID     string
	DistributionDomain string
	SiteURL            string
}

// StackExists checks if a CloudFormation stack exists.
func (c *AWSClients) StackExists(ctx context.Context, stackName string) (bool, error) {
	cf := c.CloudFormationUsEast1()

	_, err := cf.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		var notFound *types.StackNotFoundException
		if errors.As(err, &notFound) {
			return false, nil
		}
		// Also check for "does not exist" in the error message
		if isStackNotExistError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe stack: %w", err)
	}

	return true, nil
}

func isStackNotExistError(err error) bool {
	// AWS returns a generic error with "does not exist" in the message
	return err != nil && (errors.Is(err, &types.StackNotFoundException{}) ||
		(err.Error() != "" && contains(err.Error(), "does not exist")))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CreateOrUpdateStack deploys or updates the CloudFormation stack.
// Returns true if an operation was started and waiting is required.
func (c *AWSClients) CreateOrUpdateStack(ctx context.Context, params StackParams) (bool, error) {
	cf := c.CloudFormationUsEast1()
	template := GetTemplate()

	cfParams := []types.Parameter{
		{ParameterKey: aws.String("Subdomain"), ParameterValue: aws.String(params.Subdomain)},
		{ParameterKey: aws.String("DomainName"), ParameterValue: aws.String(params.DomainName)},
		{ParameterKey: aws.String("HostedZoneId"), ParameterValue: aws.String(params.HostedZoneID)},
	}

	exists, err := c.StackExists(ctx, params.StackName)
	if err != nil {
		return false, err
	}

	if exists {
		fmt.Fprintf(os.Stderr, "Updating stack: %s\n", params.StackName)
		_, err = cf.UpdateStack(ctx, &cloudformation.UpdateStackInput{
			StackName:    aws.String(params.StackName),
			TemplateBody: aws.String(template),
			Parameters:   cfParams,
			Capabilities: []types.Capability{types.CapabilityCapabilityIam},
		})
		if err != nil {
			// "No updates are to be performed" is not an error
			if isNoUpdateError(err) {
				fmt.Fprintln(os.Stderr, "Stack is up to date")
				return false, nil
			}
			return false, fmt.Errorf("failed to update stack: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Creating stack: %s\n", params.StackName)
		_, err = cf.CreateStack(ctx, &cloudformation.CreateStackInput{
			StackName:    aws.String(params.StackName),
			TemplateBody: aws.String(template),
			Parameters:   cfParams,
			Capabilities: []types.Capability{types.CapabilityCapabilityIam},
		})
		if err != nil {
			return false, fmt.Errorf("failed to create stack: %w", err)
		}
	}

	return true, nil
}

func isNoUpdateError(err error) bool {
	return err != nil && contains(err.Error(), "No updates are to be performed")
}

// WaitForStack waits for the stack to reach a stable state.
func (c *AWSClients) WaitForStack(ctx context.Context, stackName string) error {
	cf := c.CloudFormationUsEast1()

	fmt.Fprintln(os.Stderr, "Waiting for stack (this may take 10-15 minutes for certificate validation)...")

	start := time.Now()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			output, err := cf.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
				StackName: aws.String(stackName),
			})
			if err != nil {
				return fmt.Errorf("failed to describe stack: %w", err)
			}

			if len(output.Stacks) == 0 {
				return fmt.Errorf("stack not found")
			}

			stack := output.Stacks[0]
			status := stack.StackStatus
			elapsed := int(time.Since(start).Seconds())

			fmt.Fprintf(os.Stderr, "  %s [%ds]\n", status, elapsed)

			switch status {
			case types.StackStatusCreateComplete, types.StackStatusUpdateComplete:
				return nil
			case types.StackStatusCreateFailed, types.StackStatusRollbackComplete,
				types.StackStatusRollbackFailed, types.StackStatusUpdateRollbackComplete,
				types.StackStatusUpdateRollbackFailed, types.StackStatusDeleteComplete,
				types.StackStatusDeleteFailed:
				reason := c.getStackFailureReason(ctx, stackName)
				return fmt.Errorf("stack failed with status %s: %s", status, reason)
			}
			// Continue waiting for other statuses (IN_PROGRESS, etc.)
		}
	}
}

func (c *AWSClients) getStackFailureReason(ctx context.Context, stackName string) string {
	cf := c.CloudFormationUsEast1()

	events, err := cf.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return "unable to get failure reason"
	}

	for _, event := range events.StackEvents {
		if event.ResourceStatus == types.ResourceStatusCreateFailed ||
			event.ResourceStatus == types.ResourceStatusUpdateFailed {
			if event.ResourceStatusReason != nil {
				return *event.ResourceStatusReason
			}
		}
	}

	return "unknown reason"
}

// GetStackOutputs retrieves the outputs from a deployed stack.
func (c *AWSClients) GetStackOutputs(ctx context.Context, stackName string) (*StackOutputs, error) {
	cf := c.CloudFormationUsEast1()

	output, err := cf.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe stack: %w", err)
	}

	if len(output.Stacks) == 0 {
		return nil, fmt.Errorf("stack not found")
	}

	stack := output.Stacks[0]
	outputs := &StackOutputs{}

	for _, out := range stack.Outputs {
		if out.OutputKey == nil || out.OutputValue == nil {
			continue
		}
		switch *out.OutputKey {
		case "BucketName":
			outputs.BucketName = *out.OutputValue
		case "DistributionId":
			outputs.DistributionID = *out.OutputValue
		case "DistributionDomain":
			outputs.DistributionDomain = *out.OutputValue
		case "SiteURL":
			outputs.SiteURL = *out.OutputValue
		}
	}

	return outputs, nil
}

// InvalidateDistribution creates a CloudFront invalidation for all paths
// and waits for it to complete.
func (c *AWSClients) InvalidateDistribution(ctx context.Context, distributionID string) error {
	fmt.Fprintln(os.Stderr, "Creating CloudFront invalidation...")

	callerRef := fmt.Sprintf("ansel-%d", time.Now().UnixNano())

	result, err := c.CloudFront.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{
		DistributionId: aws.String(distributionID),
		InvalidationBatch: &cftypes.InvalidationBatch{
			CallerReference: aws.String(callerRef),
			Paths: &cftypes.Paths{
				Quantity: aws.Int32(1),
				Items:    []string{"/*"},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create invalidation: %w", err)
	}

	invalidationID := *result.Invalidation.Id
	fmt.Fprintf(os.Stderr, "Invalidation %s created, waiting for completion...\n", invalidationID)

	start := time.Now()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			inv, err := c.CloudFront.GetInvalidation(ctx, &cloudfront.GetInvalidationInput{
				DistributionId: aws.String(distributionID),
				Id:             aws.String(invalidationID),
			})
			if err != nil {
				return fmt.Errorf("failed to get invalidation status: %w", err)
			}

			status := *inv.Invalidation.Status
			elapsed := int(time.Since(start).Seconds())
			if status == "Completed" {
				fmt.Fprintf(os.Stderr, "Invalidation completed in %ds\n", elapsed)
				return nil
			}
			fmt.Fprintf(os.Stderr, "  %s [%ds]\n", status, elapsed)
		}
	}
}
