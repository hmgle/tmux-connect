package tagb

const (
	MetaManaged          = "@tagb_managed"
	MetaMode             = "@tagb_mode"
	MetaAgent            = "@tagb_agent"
	MetaLabel            = "@tagb_label"
	MetaCreatedBy        = "@tagb_created_by"
	MetaLastActivityUnix = "@tagb_last_activity_unix"

	ModeRelay             = "relay"
	CreatedByManualAttach = "manual-attach"
)

func MetadataKeys() []string {
	return []string{
		MetaManaged,
		MetaMode,
		MetaAgent,
		MetaLabel,
		MetaCreatedBy,
		MetaLastActivityUnix,
	}
}
