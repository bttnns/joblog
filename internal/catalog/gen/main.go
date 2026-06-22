//go:build ignore

// Command gen builds internal/catalog/companies.csv.gz, the embedded company
// snapshot. It reads the public jobhive companies CSV (ats,name,slug,url), drops
// the redundant url column (the catalog rebuilds it from ats+slug) and every
// join_com row (a large aggregator, not a real ATS company), and writes the
// remaining ats,name,slug rows gzipped. It is a dev-time tool run by hand to
// refresh the snapshot (`make catalog`); the jl binary never runs it.
//
// Source: by default it downloads
// https://storage.stapply.ai/jobhive/v1/companies.csv. Pass -in <path> to read a
// local copy instead, and -out <path> to change the output (defaults to the
// embedded artifact next to this generator's package).
package main

import (
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const sourceURL = "https://storage.stapply.ai/jobhive/v1/companies.csv"

// dropATS lists ATS values to exclude entirely. join_com is a ~23k-row aggregator
// that is not a per-company ATS and would bloat the artifact.
var dropATS = map[string]bool{"join_com": true}

func main() {
	in := flag.String("in", "", "local companies CSV path (default: download from the source URL)")
	out := flag.String("out", defaultOut(), "output gzipped path")
	flag.Parse()

	var r io.ReadCloser
	if *in != "" {
		f, err := os.Open(*in)
		if err != nil {
			log.Fatalf("open %s: %v", *in, err)
		}
		r = f
	} else {
		log.Printf("downloading %s", sourceURL)
		resp, err := http.Get(sourceURL)
		if err != nil {
			log.Fatalf("download: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("download: status %s", resp.Status)
		}
		r = resp.Body
	}
	defer r.Close()

	rows, err := readRows(r)
	if err != nil {
		log.Fatalf("read csv: %v", err)
	}

	// Deterministic order so the artifact diffs cleanly across refreshes.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][0] != rows[j][0] {
			return rows[i][0] < rows[j][0]
		}
		return rows[i][2] < rows[j][2] // by slug within an ATS
	})

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create %s: %v", *out, err)
	}
	defer f.Close()
	gw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	for _, row := range rows {
		// ats,name,slug per line. Names with commas are left as-is; the reader in
		// catalog.go splits on the first and last comma so an embedded comma in the
		// name survives.
		if _, err := fmt.Fprintf(gw, "%s,%s,%s\n", row[0], row[1], row[2]); err != nil {
			log.Fatalf("write: %v", err)
		}
	}
	if err := gw.Close(); err != nil {
		log.Fatalf("gzip close: %v", err)
	}
	fi, _ := f.Stat()
	log.Printf("wrote %s: %d rows, %d bytes", *out, len(rows), fi.Size())
}

// readRows parses the source CSV and returns [ats,name,slug] rows, skipping the
// header, dropped ATS values, and any row missing an ats or slug. The url column
// is discarded.
func readRows(r io.Reader) ([][3]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // tolerate ragged rows defensively
	var out [][3]string
	first := true
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if first {
			first = false
			if len(rec) > 0 && rec[0] == "ats" {
				continue // header
			}
		}
		if len(rec) < 3 {
			continue
		}
		ats := strings.TrimSpace(rec[0])
		name := strings.TrimSpace(rec[1])
		slug := strings.TrimSpace(rec[2])
		if ats == "" || slug == "" || dropATS[ats] {
			continue
		}
		// Strip newlines from a stray name so one row stays one line.
		name = strings.ReplaceAll(name, "\n", " ")
		name = strings.ReplaceAll(name, "\r", " ")
		out = append(out, [3]string{ats, name, slug})
	}
	return out, nil
}

// defaultOut points at internal/catalog/companies.csv.gz, derived from this
// source file's own location so the generator works from any working directory.
func defaultOut() string {
	_, this, _, ok := runtime.Caller(0)
	if !ok {
		return "companies.csv.gz"
	}
	return filepath.Join(filepath.Dir(filepath.Dir(this)), "companies.csv.gz")
}
