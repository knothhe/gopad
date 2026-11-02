//go:build !darwin

package launchview

func nativeScrollPhase() scrollPhase { return scrollUnphased }
