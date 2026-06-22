package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// scrapeHint tells the user how to install the documented default producer when
// the configured scraper binary is not on PATH.
const scrapeHint = "install a scraper, e.g.: uv tool install jobhive-py"

// scraperArgv expands a scraper command template into an argv by substituting
// {ats} and {slug}, then splitting on whitespace. The template (for example the
// default "jobhive scrape {ats} {slug} --format json") is space-split, so the
// substituted values must not contain spaces; ATS slugs never do. An empty
// template yields an error rather than an empty argv.
func scraperArgv(template, ats, slug string) ([]string, error) {
	r := strings.NewReplacer("{ats}", ats, "{slug}", slug)
	argv := strings.Fields(r.Replace(template))
	if len(argv) == 0 {
		return nil, fmt.Errorf("scraper command template is empty")
	}
	return argv, nil
}

// scrape runs the configured producer for one company and returns its stdout
// (the ATS job JSON that role import ingests). jl itself makes no network calls;
// the producer does the HTTP. On a nonzero exit the error wraps the command's
// stderr; when the binary is missing the error includes the install hint.
func scrape(template, ats, slug string) ([]byte, error) {
	argv, err := scraperArgv(template, ats, slug)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("scraper %q not found on PATH; %s", argv[0], scrapeHint)
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return nil, fmt.Errorf("%s: %w", argv[0], err)
	}
	return stdout.Bytes(), nil
}
