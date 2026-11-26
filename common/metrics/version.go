package metrics

// VersionString the current release version
const VersionString = "1.2.5"

var (
	// Revision is the VCS revision associated with this build. Overridden using ldflags
	// at compile time. Example:
	// $ go build -ldflags "-X github.com/cadence-workflow/shard-manager/common/metrics.Revision=abcdef" ...
	// see get-ldflags.sh for GIT_REVISION
	Revision = "unknown"

	// Branch is the VCS branch associated with this build.
	Branch = "unknown"

	// ReleaseVersion is the version associated with this build.
	ReleaseVersion = "unknown"

	// BuildDate is the date this build was created.
	BuildDate = "unknown"

	// BuildTimeUnix is the seconds since epoch representing the date this build was created.
	BuildTimeUnix = "0"
)
