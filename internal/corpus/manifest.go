package corpus

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// Manifest is a _links entry: an assertion about a subject, written beside it
// rather than inside it. The shape is one file per subject, named by its
// subject, carrying the same links: list an artifact writes — see
// docs/decision/0030.
//
// Two subjects need it. One cannot speak for itself at all: an image, a PDF, a
// binary fixture has no frontmatter to put a list in. The other can, but the
// person asserting the link is not its author — a third party must be able to
// say that something relates to an artifact without editing that artifact.
//
//	# docs/_links/cover.yaml
//	subject: canon/cover.png
//	links:
//	  - { target: canon:kael-eyes-are-green, relation: about }
type Manifest struct {
	// Path is the manifest file, relative to the corpus root.
	Path string
	// Subject is what the links belong to, as authored. Like a target it is an
	// artifact, a workspace file, or a URI.
	Subject string
	// Reference is Subject read against the corpus. It is filled in when links
	// are resolved, because only then does the corpus know its own types.
	Reference Reference
	Links     []model.Link
}

type manifestFile struct {
	Subject string `yaml:"subject"`
	Links   []any  `yaml:"links"`
}

// loadManifests reads every manifest under _links. A manifest that cannot be
// read is a diagnostic and not a load error: it names no artifact's identity,
// so nothing about the corpus's map from id to artifact depends on it.
func loadManifests(root string) ([]Manifest, []Problem) {
	dir := filepath.Join(root, LinksDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Problem{{Kind: Unreadable, Path: LinksDir, Detail: err.Error()}}
	}

	var out []Manifest
	var problems []Problem
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(LinksDir, e.Name()))
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			problems = append(problems, Problem{Kind: Unreadable, Path: rel, Detail: err.Error()})
			continue
		}
		var f manifestFile
		if err := unmarshalYAML(src, &f); err != nil {
			problems = append(problems, Problem{Kind: Unreadable, Path: rel, Detail: firstLine(err.Error())})
			continue
		}

		subject := strings.TrimSpace(f.Subject)
		if subject == "" {
			// The filename is the fallback, never the identity — the same rule
			// artifacts follow. It lets a manifest sit beside a file of the
			// same name without repeating it.
			subject = strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		}

		links, malformed := linksFromFrontmatter(toAnyList(f.Links))
		for _, m := range malformed {
			problems = append(problems, Problem{Kind: MalformedLink, Path: rel, Detail: m})
		}
		out = append(out, Manifest{Path: rel, Subject: subject, Links: links})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, problems
}

func toAnyList(items []any) any {
	if items == nil {
		return nil
	}
	return items
}
