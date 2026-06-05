package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
)

type searchHit struct {
	summary domain.GraphNodeSummary
	score   float64
}

func (s *Service) searchAll(ctx context.Context, projectID, query string, limit int) ([]searchHit, []domain.GraphWarning, error) {
	if len(s.hydrators) == 0 {
		return []searchHit{}, []domain.GraphWarning{}, nil
	}

	type result struct {
		hits    []searchHit
		warning *domain.GraphWarning
		err     error
	}

	normalizedQuery := strings.TrimSpace(strings.ToLower(query))
	typesSeen := make([]string, 0, len(s.hydrators))
	for nodeType := range s.hydrators {
		typesSeen = append(typesSeen, nodeType)
	}
	sort.Strings(typesSeen)

	results := make(chan result, len(typesSeen))
	var wg sync.WaitGroup
	for _, nodeType := range typesSeen {
		hydrator := s.hydrators[nodeType]
		wg.Add(1)
		go func(nodeType string, hydrator domain.NodeHydrator) {
			defer wg.Done()

			searchCtx, cancel := context.WithTimeout(ctx, s.searchTimeout)
			defer cancel()

			summaries, err := hydrator.Search(searchCtx, projectID, query, limit)
			if err != nil {
				if searchCtx.Err() == context.DeadlineExceeded || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
					w := warningWithCappedIDs(
						"search_partial_timeout",
						fmt.Sprintf("search timeout for node_type=%q", nodeType),
						nodeType,
						nil,
					)
					results <- result{warning: &w}
					return
				}
				results <- result{err: err}
				return
			}

			hits := make([]searchHit, 0, len(summaries))
			for idx := range summaries {
				summary := summaries[idx]
				summary.Type = normalizeNodeType(summary.Type)
				if summary.Type == "" {
					summary.Type = nodeType
				}
				createdAt, parseErr := parseRFC3339Strict(summary.CreatedAt, fmt.Sprintf("%s/%s", summary.Type, summary.ID))
				if parseErr != nil {
					results <- result{err: parseErr}
					return
				}
				summary.CreatedAt = createdAt.Format(time.RFC3339Nano)
				hits = append(hits, searchHit{
					summary: summary,
					score:   searchScore(summary, normalizedQuery),
				})
			}
			results <- result{hits: hits}
		}(nodeType, hydrator)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var warnings []domain.GraphWarning
	allHits := make([]searchHit, 0, len(s.hydrators)*max(limit, 1))
	for item := range results {
		if item.err != nil {
			return nil, warningsOrEmpty(warnings), item.err
		}
		if item.warning != nil {
			warnings = append(warnings, *item.warning)
			continue
		}
		allHits = append(allHits, item.hits...)
	}

	sortSearchHits(allHits)
	if len(allHits) > limit {
		allHits = allHits[:limit]
	}
	if allHits == nil {
		allHits = []searchHit{}
	}
	return allHits, warningsOrEmpty(warnings), nil
}

func summariesFromHits(hits []searchHit) []domain.GraphNodeSummary {
	if len(hits) == 0 {
		return []domain.GraphNodeSummary{}
	}
	out := make([]domain.GraphNodeSummary, 0, len(hits))
	for _, hit := range hits {
		out = append(out, hit.summary)
	}
	return out
}

func sortSearchHits(hits []searchHit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		leftTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(hits[i].summary.CreatedAt))
		rightTime, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(hits[j].summary.CreatedAt))
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		leftType := strings.TrimSpace(hits[i].summary.Type)
		rightType := strings.TrimSpace(hits[j].summary.Type)
		if leftType != rightType {
			return leftType < rightType
		}
		return strings.TrimSpace(hits[i].summary.ID) < strings.TrimSpace(hits[j].summary.ID)
	})
}

func sortSummariesBySearchScore(summaries []domain.GraphNodeSummary, query string) {
	normalizedQuery := strings.TrimSpace(strings.ToLower(query))
	hits := make([]searchHit, 0, len(summaries))
	for _, summary := range summaries {
		hits = append(hits, searchHit{
			summary: summary,
			score:   searchScore(summary, normalizedQuery),
		})
	}
	sortSearchHits(hits)
	for idx := range hits {
		summaries[idx] = hits[idx].summary
	}
}

func searchScore(summary domain.GraphNodeSummary, normalizedQuery string) float64 {
	title := strings.ToLower(strings.TrimSpace(summary.Title))
	if normalizedQuery == "" {
		return 0.4
	}
	if title == normalizedQuery {
		return 1.0
	}
	if strings.HasPrefix(title, normalizedQuery) {
		return 0.8
	}
	if strings.Contains(title, normalizedQuery) {
		return 0.6
	}
	tokens := searchTokens(normalizedQuery)
	matches := queryTokenMatchCount(tokens, title)
	if matches > 0 {
		return 0.4 + (0.15 * float64(matches) / float64(max(len(tokens), 1)))
	}
	return 0.4
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
