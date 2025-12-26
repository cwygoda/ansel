package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cwygoda/ansel/internal/nanoid"
	"github.com/cwygoda/ansel/internal/publish"
	"github.com/spf13/cobra"
)

var publishCmd = &cobra.Command{
	Use:   "publish [flags]",
	Short: "Publish static files to a CDN-backed subdomain",
	Long: `Publish static files to AWS CloudFront with automatic SSL.

Creates a CloudFormation stack with:
  - S3 bucket for content storage
  - CloudFront distribution with OAC
  - ACM certificate (auto-validated via DNS)
  - Route53 subdomain record

On first run, a random subdomain is generated and saved to .ansel.toml.
Subsequent runs update the existing site.

Requires AWS credentials configured via:
  - AWS CLI profile (~/.aws/credentials)
  - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
  - IAM role (when running on EC2/ECS)

Examples:
  # Publish ./build directory (default)
  ansel publish

  # Publish a specific directory
  ansel publish --build-dir ./dist

  # Use a specific subdomain
  ansel publish --subdomain gallery

  # Use a specific AWS profile
  ansel publish --profile myprofile`,
	RunE: runPublish,
}

var (
	publishSubdomain string
	publishBuildDir  string
	publishProfile   string
	publishRegion    string
)

func init() {
	rootCmd.AddCommand(publishCmd)

	publishCmd.Flags().StringVar(&publishSubdomain, "subdomain", "", "Subdomain name (generated if not provided)")
	publishCmd.Flags().StringVar(&publishBuildDir, "build-dir", "./build", "Directory containing files to upload")
	publishCmd.Flags().StringVar(&publishProfile, "profile", "", "AWS profile name")
	publishCmd.Flags().StringVar(&publishRegion, "region", "", "AWS region (default from AWS config)")
}

func runPublish(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Verify build directory exists
	info, err := os.Stat(publishBuildDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("build directory not found: %s", publishBuildDir)
		}
		return fmt.Errorf("failed to access build directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", publishBuildDir)
	}

	// Load project config
	cfg, err := publish.LoadProjectConfig()
	if err != nil {
		return err
	}

	// Initialize AWS clients
	fmt.Fprintln(os.Stderr, "Initializing AWS...")
	clients, err := publish.NewAWSClients(ctx, publishProfile, publishRegion)
	if err != nil {
		return fmt.Errorf("failed to initialize AWS: %w", err)
	}

	// Get hosted zone (from config or discover)
	var zone *publish.HostedZone
	if cfg.Publish.HostedZoneID != "" && cfg.Publish.DomainName != "" {
		zone = &publish.HostedZone{
			ID:   cfg.Publish.HostedZoneID,
			Name: cfg.Publish.DomainName,
		}
		fmt.Fprintf(os.Stderr, "Using saved zone: %s\n", zone.Name)
	} else {
		fmt.Fprintln(os.Stderr, "Checking Route53 hosted zones...")
		zones, err := clients.ListHostedZones(ctx)
		if err != nil {
			return err
		}

		zone, err = publish.SelectHostedZone(zones)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Using zone: %s\n", zone.Name)

		// Save zone to config for next time
		cfg.Publish.HostedZoneID = zone.ID
		cfg.Publish.DomainName = zone.Name
	}

	// Determine subdomain
	subdomain := publishSubdomain
	if subdomain == "" {
		subdomain = cfg.Publish.Subdomain
	}
	if subdomain == "" {
		subdomain, err = nanoid.Generate()
		if err != nil {
			return fmt.Errorf("failed to generate subdomain: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Generated subdomain: %s\n", subdomain)
	}

	// Save config if anything changed
	if cfg.Publish.Subdomain != subdomain || cfg.Publish.HostedZoneID != zone.ID {
		cfg.Publish.Subdomain = subdomain
		cfg.Publish.HostedZoneID = zone.ID
		cfg.Publish.DomainName = zone.Name
		if err := publish.SaveProjectConfig(cfg); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Saved configuration to .ansel.toml")
	}

	// Create/update CloudFormation stack
	stackName := fmt.Sprintf("ansel-%s", subdomain)
	stackParams := publish.StackParams{
		StackName:    stackName,
		Subdomain:    subdomain,
		DomainName:   zone.Name,
		HostedZoneID: zone.ID,
	}

	needsWait, err := clients.CreateOrUpdateStack(ctx, stackParams)
	if err != nil {
		return err
	}

	// Wait for stack to complete only if an operation was started
	if needsWait {
		if err := clients.WaitForStack(ctx, stackName); err != nil {
			return err
		}
	}

	// Get stack outputs
	outputs, err := clients.GetStackOutputs(ctx, stackName)
	if err != nil {
		return err
	}

	// Sync files to S3
	uploaded, err := clients.SyncDirectory(ctx, outputs.BucketName, publishBuildDir)
	if err != nil {
		return err
	}

	// Invalidate CloudFront cache if any files were uploaded
	if uploaded > 0 {
		if err := clients.InvalidateDistribution(ctx, outputs.DistributionID); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "\nSite published: %s\n", outputs.SiteURL)
	return nil
}
