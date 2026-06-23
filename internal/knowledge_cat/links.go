package knowledge_cat

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// -- Link resolution -----------------------------------------------------------

// ResolvedLink is the result of resolving a raw markdown link target to a
// concept ID. It carries flags for special link types so callers don't need
// to duplicate detection logic.
type ResolvedLink struct {
	// ConceptID is the resolved concept ID, or empty for external/fragment links.
	ConceptID string
	// Fragment is the #block-id portion of the link, if any.
	Fragment string
	// External is true when the link is an http/https URL.
	External bool
	// ExternalURL holds the original URL when External is true.
	ExternalURL string
}

// resolveLinkTarget resolves a raw markdown link target relative to a source
// concept ID. It handles:
//   - External URLs (http://, https://)
//   - Fragment-only links (#block-id)
//   - Absolute paths (/path/to/concept.md)
//   - Relative paths (./other.md, ../other.md)
//   - Fragment extraction (target.md#block)
func resolveLinkTarget(linkTarget, sourceID string) ResolvedLink {
	target := strings.TrimSpace(linkTarget)

	// External URLs.
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return ResolvedLink{External: true, ExternalURL: target}
	}

	// Fragment-only link (#block-id) — stays within source concept.
	if strings.HasPrefix(target, "#") {
		return ResolvedLink{Fragment: strings.TrimPrefix(target, "#")}
	}

	// Extract fragment before stripping suffixes.
	fragment := ""
	if idx := strings.Index(target, "#"); idx >= 0 {
		fragment = target[idx+1:]
		target = target[:idx]
	}

	// Strip .md suffix if present.
	target = strings.TrimSuffix(target, ".md")

	// Strip trailing slash (directory references like "subdir/").
	target = strings.TrimSuffix(target, "/")

	// Absolute links.
	if strings.HasPrefix(target, "/") {
		return ResolvedLink{ConceptID: strings.TrimPrefix(target, "/"), Fragment: fragment}
	}

	// Relative links — resolve relative to source concept's directory.
	sourceDir := filepath.Dir(sourceID)
	if sourceDir == "." {
		return ResolvedLink{ConceptID: target, Fragment: fragment}
	}
	resolved := filepath.Join(sourceDir, target)
	resolved = filepath.Clean(resolved)
	return ResolvedLink{ConceptID: resolved, Fragment: fragment}
}

// -- Link graph types ----------------------------------------------------------

// LinkTarget represents a resolved link target from one concept to another.
type LinkTarget struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Fragment string `json:"fragment,omitempty"`
}

// LinksResult holds the link graph for a single concept.
type LinksResult struct {
	ConceptID    string       `json:"concept_id"`
	ConceptTitle string       `json:"concept_title"`
	ConceptType  string       `json:"concept_type"`
	Outgoing     []LinkTarget `json:"outgoing"`
	Incoming     []LinkTarget `json:"incoming"`
}

// BrokenLink describes a broken outgoing link from a concept.
type BrokenLink struct {
	SourceID   string `json:"source_id"`
	LinkTarget string `json:"link_target"`
	ResolvedID string `json:"resolved_id"`
}

// LinkFollowResult holds the result of following a markdown link.
type LinkFollowResult struct {
	SourceID    string   `json:"source_id"`
	LinkTarget  string   `json:"link_target"`
	ResolvedID  string   `json:"resolved_id"`
	Target      *Concept `json:"-"`
	Fragment    string   `json:"fragment,omitempty"`
	SameConcept bool     `json:"same_concept"`
	External    bool     `json:"external"`
	ExternalURL string   `json:"external_url,omitempty"`
	Broken      bool     `json:"broken"`
}

// -- Link operations -----------------------------------------------------------

// FollowLink resolves a raw markdown link target from a source concept and
// returns the linked concept.
func (b *Bundle) FollowLink(sourceID, linkTarget string) (*LinkFollowResult, error) {
	source := b.GetConcept(sourceID)
	if source == nil {
		return nil, fmt.Errorf("follow-link: source concept %q not found", sourceID)
	}

	resolved := resolveLinkTarget(linkTarget, sourceID)

	result := &LinkFollowResult{
		SourceID:   sourceID,
		LinkTarget: linkTarget,
		ResolvedID: resolved.ConceptID,
	}

	if resolved.External {
		result.External = true
		result.ExternalURL = resolved.ExternalURL
		return result, nil
	}

	if resolved.ConceptID == "" {
		// Fragment-only link.
		result.SameConcept = true
		result.Fragment = resolved.Fragment
		result.Target = source
		return result, nil
	}

	result.Fragment = resolved.Fragment

	target, ok := b.Concepts[resolved.ConceptID]
	if !ok {
		result.Broken = true
		return result, fmt.Errorf("follow-link: link to %q (resolved to %q) is broken — concept not found",
			linkTarget, resolved.ConceptID)
	}

	result.Target = target
	return result, nil
}

// LinksOf returns the link graph for a concept.
func (b *Bundle) LinksOf(conceptID string) (*LinksResult, error) {
	c := b.GetConcept(conceptID)
	if c == nil {
		return nil, fmt.Errorf("links: concept %q not found", conceptID)
	}

	result := &LinksResult{
		ConceptID:    c.ID,
		ConceptTitle: c.Title,
		ConceptType:  c.Type,
	}

	// Phase 1: outgoing links.
	outgoing := make([]LinkTarget, 0, len(c.Links))
	for _, link := range c.Links {
		resolved := resolveLinkTarget(link, c.ID)
		if resolved.External || resolved.ConceptID == "" {
			continue // external URL or fragment-only
		}
		if target, ok := b.Concepts[resolved.ConceptID]; ok {
			outgoing = append(outgoing, LinkTarget{
				ID:       target.ID,
				Type:     target.Type,
				Title:    target.Title,
				Fragment: resolved.Fragment,
			})
		}
	}
	sort.Slice(outgoing, func(i, j int) bool { return outgoing[i].ID < outgoing[j].ID })
	result.Outgoing = outgoing

	// Phase 2: backlinks.
	incoming := make([]LinkTarget, 0, len(b.Concepts))
	for _, other := range b.Concepts {
		if other.ID == conceptID {
			continue
		}
		for _, link := range other.Links {
			resolved := resolveLinkTarget(link, other.ID)
			if resolved.ConceptID == conceptID {
				incoming = append(incoming, LinkTarget{
					ID:    other.ID,
					Type:  other.Type,
					Title: other.Title,
				})
				break
			}
		}
	}
	sort.Slice(incoming, func(i, j int) bool { return incoming[i].ID < incoming[j].ID })
	result.Incoming = incoming

	return result, nil
}

// -- Broken-link detection (unified) ------------------------------------------

// findBrokenLinks returns all broken outgoing links from one concept in an
// already-opened bundle.
func findBrokenLinks(b *Bundle, conceptID string) []BrokenLink {
	c := b.GetConcept(conceptID)
	if c == nil {
		return nil
	}
	var broken []BrokenLink
	for _, link := range c.Links {
		resolved := resolveLinkTarget(link, c.ID)
		if resolved.External || resolved.ConceptID == "" {
			continue
		}
		if _, ok := b.Concepts[resolved.ConceptID]; !ok {
			broken = append(broken, BrokenLink{
				SourceID:   c.ID,
				LinkTarget: link,
				ResolvedID: resolved.ConceptID,
			})
		}
	}
	return broken
}

// CheckConceptLinks checks outgoing links from one concept, re-opening the
// bundle from disk.
func CheckConceptLinks(bundlePath, conceptID string) ([]BrokenLink, error) {
	b, err := Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("check links for %s: %w", conceptID, err)
	}
	if b.GetConcept(conceptID) == nil {
		return nil, fmt.Errorf("check links: concept %q not found", conceptID)
	}
	return findBrokenLinks(b, conceptID), nil
}

// CheckAllLinks checks cross-links across every concept in the bundle.
func CheckAllLinks(bundlePath string) ([]BrokenLink, error) {
	b, err := Open(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("check all links: %w", err)
	}
	var broken []BrokenLink
	for id := range b.Concepts {
		broken = append(broken, findBrokenLinks(b, id)...)
	}
	return broken, nil
}
