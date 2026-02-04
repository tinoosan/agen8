package resources

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"
)

func bestMatchInFile(ctx context.Context, absPath string, re *regexp.Regexp, reOK bool, qLower string) (bestMatch, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return bestMatch{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	lineN := 0
	var best bestMatch
	for sc.Scan() {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return best, ctx.Err()
			default:
			}
		}

		lineN++
		ln := sc.Text()
		match := false
		matchN := 0
		if reOK && re != nil {
			idxs := re.FindAllStringIndex(ln, -1)
			if len(idxs) != 0 {
				match = true
				matchN = len(idxs)
			}
		} else {
			if strings.Contains(strings.ToLower(ln), qLower) {
				match = true
				matchN = 1
			}
		}
		if !match {
			continue
		}
		score := float64(matchN)
		if score > best.score {
			best = bestMatch{
				score: score,
				line:  lineN,
				text:  strings.TrimSpace(ln),
			}
		}
	}
	return best, sc.Err()
}
