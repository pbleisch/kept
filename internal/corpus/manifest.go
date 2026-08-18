package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pbleisch/kept/internal/model"
)

// Manifest is a _links entry: the links of something that cannot carry its own.
// An image, a PDF and a binary fixture have no frontmatter to put a links: list
// in, and citation is how something without standing acquires force, so
// something has to be able to say what they point at.
//
// No document fixes the file's shape, so this is the shape it takes here: one
// manifest per subject, named by its subject, with the same links: list an
// artifact writes. One file per subject rather than one shared index, because
// every tracked format has to merge without a human and two subjects must never
// want the same file.
//
//	# docs/_links/cover.yaml
//	subject: attachments/cover.png
//	links:
//	  - { target: canon:kael-eyes-are-green, relation: about }
type Manifest struct {
	// Path is the manifest file, relative to the corpus root.
	Path string
	// Subject is what the links belong to: a path inside the corpus, or a
	// "type:slug" reference to an artifact that would rather keep its links
	// beside it than inside it.
	Subject string
	Links   []model.Link
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

// checkSubject reports a manifest whose subject is neither an artifact nor a
// file inside the corpus. Containment is checked rather than assumed: a subject
// of ../../etc/passwd is refused here rather than followed, which is the
// traversal protection 0002 asks for.
func (m Manifest) checkSubject(root string, isArtifact func(string) bool) (Problem, bool) {
	if isArtifact(m.Subject) {
		return Problem{}, false
	}
	if strings.Contains(m.Subject, ":") {
		return Problem{
			Kind: ManifestSubject, Path: m.Path,
			Detail: fmt.Sprintf("subject %q reads as an artifact reference and no artifact claims it", m.Subject),
		}, true
	}

	full := filepath.Join(root, filepath.FromSlash(m.Subject))
	inside, err := filepath.Rel(root, full)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return Problem{
			Kind: ManifestSubject, Path: m.Path,
			Detail: fmt.Sprintf("subject %q resolves outside the corpus root", m.Subject),
		}, true
	}
	if _, err := os.Stat(full); err != nil {
		return Problem{
			Kind: ManifestSubject, Path: m.Path,
			Detail: fmt.Sprintf("subject %q is neither an artifact nor a file in the corpus", m.Subject),
		}, true
	}
	return Problem{}, false
}
