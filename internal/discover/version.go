package discover

import "strconv"

// Minimum Artifactory versions at which the Access API v2 replacements for
// the deprecated Artifactory Security API become available (per JFrog's
// "Deprecated JFrog APIs" reference): users/groups landed together in
// 7.49.3, permission targets followed later in 7.72.0. Below these, the
// v2 endpoints 404 and GetUsers/GetGroups/GetPermissionTargets go straight
// to the legacy Artifactory Security API instead of probing first.
var (
	usersGroupsV2MinVersion = [3]int{7, 49, 3}
	permissionsV2MinVersion = [3]int{7, 72, 0}
)

// versionAtLeast reports whether an Artifactory version string (e.g.
// "7.31.13") is >= major.minor.patch. An empty or unparsable version
// (custom builds, a version fetch that itself failed) returns false, so
// callers default to the legacy Artifactory Security API, which has
// worked on every supported version since 6.23.21.
func versionAtLeast(version string, min [3]int) bool {
	parts := parseVersion(version)
	if parts == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if parts[i] != min[i] {
			return parts[i] > min[i]
		}
	}
	return true
}

// parseVersion reads up to the first three dot-separated numeric
// components of an Artifactory version string, tolerating a non-numeric
// suffix on the last one (e.g. "3-hf1"). Returns nil if version doesn't
// start with a parsable numeric component.
func parseVersion(version string) *[3]int {
	var out [3]int
	field := 0
	start := 0
	for i := 0; i <= len(version); i++ {
		if i == len(version) || version[i] == '.' {
			if field >= 3 {
				break
			}
			numEnd := start
			for numEnd < i && version[numEnd] >= '0' && version[numEnd] <= '9' {
				numEnd++
			}
			if numEnd == start {
				return nil
			}
			n, err := strconv.Atoi(version[start:numEnd])
			if err != nil {
				return nil
			}
			out[field] = n
			field++
			start = i + 1
		}
	}
	if field == 0 {
		return nil
	}
	return &out
}
