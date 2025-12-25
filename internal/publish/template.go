package publish

import _ "embed"

//go:embed template.yaml
var cloudFormationTemplate string

// GetTemplate returns the CloudFormation template.
func GetTemplate() string {
	return cloudFormationTemplate
}
