package publish

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"golang.org/x/term"
)

// HostedZone represents a Route53 hosted zone.
type HostedZone struct {
	ID   string
	Name string
}

// ListHostedZones returns all public hosted zones in the account.
func (c *AWSClients) ListHostedZones(ctx context.Context) ([]HostedZone, error) {
	var zones []HostedZone

	paginator := route53.NewListHostedZonesPaginator(c.Route53, &route53.ListHostedZonesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list hosted zones: %w", err)
		}

		for _, hz := range page.HostedZones {
			// Skip private hosted zones
			if hz.Config != nil && hz.Config.PrivateZone {
				continue
			}

			// Extract ID from "/hostedzone/Z123..." format
			id := strings.TrimPrefix(*hz.Id, "/hostedzone/")

			// Remove trailing dot from domain name
			name := strings.TrimSuffix(*hz.Name, ".")

			zones = append(zones, HostedZone{
				ID:   id,
				Name: name,
			})
		}
	}

	return zones, nil
}

// SelectHostedZone handles the zone selection logic:
// - 0 zones: returns error
// - 1 zone: returns it
// - N zones: prompts user to select
func SelectHostedZone(zones []HostedZone) (*HostedZone, error) {
	if len(zones) == 0 {
		return nil, fmt.Errorf("no Route53 hosted zone found")
	}

	if len(zones) == 1 {
		return &zones[0], nil
	}

	// Multiple zones - need to prompt
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, fmt.Errorf("multiple hosted zones found; run interactively or specify zone in .ansel.toml")
	}

	fmt.Fprintln(os.Stderr, "Multiple hosted zones found. Select one:")
	for i, z := range zones {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, z.Name)
	}
	fmt.Fprint(os.Stderr, "Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	num, err := strconv.Atoi(input)
	if err != nil || num < 1 || num > len(zones) {
		return nil, fmt.Errorf("invalid selection: %s", input)
	}

	return &zones[num-1], nil
}
