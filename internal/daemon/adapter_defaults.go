package daemon

type adapterBehavior struct {
	platform      string
	commandPrefix string
}

func newAdapterBehavior(platform string, commandPrefix string) adapterBehavior {
	return adapterBehavior{
		platform:      normalizePlatformName(platform),
		commandPrefix: commandPrefix,
	}
}

func (b adapterBehavior) PromptText(_ IncomingMessage, spec commandPromptSpec) string {
	return spec.Message
}

func (b adapterBehavior) NormalizeSnapshotMode(mode snapshotMode) snapshotMode {
	return defaultSnapshotMode(mode)
}

func (b adapterBehavior) SnapshotCaption(paneKey string) string {
	return formatSnapshotCaption(paneKey)
}

func (b adapterBehavior) HelpText() string {
	return platformHelpText(b.platform, b.commandPrefix)
}
