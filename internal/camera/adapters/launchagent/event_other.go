//go:build !darwin

package launchagent

func ConsumeIOKitEvent(timeoutSeconds int) bool { return false }
