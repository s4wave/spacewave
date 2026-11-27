package prom_plugin

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

// maxScrapeBytes bounds the body of one scrape.
const maxScrapeBytes = 64 << 20

// fallbackContentType is the Prometheus text format used to parse a page that
// sends no usable Content-Type.
const fallbackContentType = "text/plain"

// httpSource returns the source that fetches and parses the metrics page at
// url. The scrape's context bounds each request.
func httpSource(url string) scrapeSource {
	client := &http.Client{}
	return func(ctx context.Context) ([]scrapeSample, error) {
		// Request the page.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "text/plain;version=0.0.4;q=0.9,*/*;q=0.1")

		// Send it and require a page.
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errors.Errorf("server returned HTTP status %s", resp.Status)
		}

		// Read the bounded body.
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeBytes+1))
		if err != nil {
			return nil, err
		}
		if len(body) > maxScrapeBytes {
			return nil, errors.Errorf("metrics page exceeds %d bytes", maxScrapeBytes)
		}
		return parsePage(body, resp.Header.Get("Content-Type"))
	}
}

// parsePage decodes the float samples of a metrics page. Native histograms and
// metadata entries are skipped. Exporter timestamps are ignored.
func parsePage(body []byte, contentType string) ([]scrapeSample, error) {
	// Choose the parser from the content type.
	p, err := textparse.New(body, contentType, nil, textparse.ParserOptions{FallbackContentType: fallbackContentType})
	if p == nil {
		return nil, errors.Wrap(err, "metrics page format")
	}

	// Collect each float sample with its labels.
	var out []scrapeSample
	for {
		entry, err := p.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, errors.Wrap(err, "parse metrics page")
		}
		if entry != textparse.EntrySeries {
			continue
		}
		_, _, v := p.Series()
		var lset labels.Labels
		p.Labels(&lset)
		out = append(out, scrapeSample{lset: lset, v: v})
	}
}
