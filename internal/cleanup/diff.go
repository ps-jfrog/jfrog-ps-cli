package cleanup

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// DiffAQLQuery builds the AQL query used by diff-delete-artifacts to list all
// items in repo, along with their sha256, for comparing the same repo across
// two servers.
func DiffAQLQuery(repo string) string {
	return fmt.Sprintf(`items.find({"repo":"%s"}).include("path","name","sha256")`, repo)
}

// DiffKey returns the identifier used to match the same item across two
// servers, independent of which server it came from: "path/name", or just
// "name" when the item is at the root of the repo (AQL reports the root path
// as ".").
func DiffKey(m ArtifactMatch) string {
	if m.Path == "" || m.Path == "." {
		return m.Name
	}
	return m.Path + "/" + m.Name
}

// TargetOnlyArtifacts runs the diff AQL query against both source and target
// for repo, and returns the items present in target but not in source --
// i.e. the candidates for deletion from target in order to keep it in sync
// with source.
func TargetOnlyArtifacts(source, target *jfclient.Client, repo string) ([]ArtifactMatch, error) {
	query := DiffAQLQuery(repo)

	sourceItems, err := FindArtifacts(source, query)
	if err != nil {
		return nil, fmt.Errorf("query source: %w", err)
	}
	targetItems, err := FindArtifacts(target, query)
	if err != nil {
		return nil, fmt.Errorf("query target: %w", err)
	}

	sourceKeys := make(map[string]bool, len(sourceItems))
	for _, m := range sourceItems {
		sourceKeys[DiffKey(m)] = true
	}

	var onlyInTarget []ArtifactMatch
	for _, m := range targetItems {
		if !sourceKeys[DiffKey(m)] {
			onlyInTarget = append(onlyInTarget, m)
		}
	}
	return onlyInTarget, nil
}
