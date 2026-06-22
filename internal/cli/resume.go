package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bttnns/joblog/internal/model"
	"github.com/bttnns/joblog/internal/roles"
	"github.com/bttnns/joblog/internal/store"
	"github.com/dslipak/pdf"
	"github.com/spf13/cobra"
)

func init() { addCommand(newResumeCmd) }

// newResumeCmd groups the resume verbs under `jl resume`. A bare `jl resume`
// runs ls. The collection holds one base resume plus one tailored variant per
// role; jl only stores the source and extracts plaintext, tailoring is the
// agent's job.
func newResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Your resumes: a base CV plus one tailored variant per role",
		Long: "Store your base resume and per-role tailored variants, each as a canonical\n" +
			"source plus an extracted plaintext .txt an agent can read cheaply without PDF\n" +
			"tooling. PDF is converted to text; Markdown and JSON pass through. jl only\n" +
			"extracts text; tailoring is the agent's job. A bare 'jl resume' lists them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResumeList(cmd)
		},
	}
	cmd.AddCommand(
		newResumeListCmd(),
		newResumeSetCmd(),
		newResumeAddCmd(),
		newResumeShowCmd(),
		newResumeDiffCmd(),
		newResumeRmCmd(),
	)
	return cmd
}

func newResumeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List the base resume and every tailored-per-role variant",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResumeList(cmd)
		},
	}
}

// resumeVariant is one row in `jl resume ls`: the base resume or a tailored
// per-role variant. Role/Company/Title are empty for the base.
type resumeVariant struct {
	Kind    string `json:"kind"` // "base" or "tailored"
	Role    string `json:"role,omitempty"`
	Company string `json:"company,omitempty"`
	Title   string `json:"title,omitempty"`
	Source  string `json:"source"` // path to the canonical source, relative to the data dir
	Text    string `json:"text"`   // path to the extracted .txt, relative to the data dir
}

func runResumeList(cmd *cobra.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		return err
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	allRoles, err := s.LoadRoles()
	if err != nil {
		return err
	}

	var variants []resumeVariant
	if cfg.ResumePath != "" {
		variants = append(variants, resumeVariant{
			Kind:   "base",
			Source: cfg.ResumePath,
			Text:   filepath.Join("resume", "resume.txt"),
		})
	}
	for _, v := range listTailoredResumes(s) {
		row := resumeVariant{Kind: "tailored", Role: v.role, Source: v.source, Text: v.text}
		row.Company = v.slug
		// The filename fragment is the sanitized role id; recover the real role by
		// matching its sanitized GlobalID so we can show the real id and title.
		if r, ok := findRoleBySanitizedID(allRoles, v.role); ok {
			row.Role = r.GlobalID
			row.Company = roleCompanySlug(r)
			row.Title = r.Title
		}
		variants = append(variants, row)
	}

	if wantJSON(cmd) {
		return emitJSON(variants)
	}
	if len(variants) == 0 {
		info("no resumes stored; run: jl resume set <file>")
		return nil
	}
	tw := newTabWriter()
	fmt.Fprintln(tw, "KIND\tROLE\tCOMPANY\tTITLE\tSOURCE")
	for _, v := range variants {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", v.Kind, v.Role, v.Company, truncate(v.Title, 40), v.Source)
	}
	return tw.Flush()
}

func newResumeSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <file.md|file.pdf|file.json>",
		Short: "Set or replace the base resume",
		Long: "Record the canonical base resume under resume/ and write resume.txt (PDF\n" +
			"extracted; Markdown and JSON pass through). This is the resume profile build\n" +
			"reads from.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			rel, n, err := setBaseResume(s, args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(map[string]any{"resume_path": s.Path(rel), "text_path": s.Path("resume", "resume.txt"), "text_bytes": n})
			}
			info("Stored resume at %s and wrote resume.txt (%d bytes)", s.Path(rel), n)
			return nil
		},
	}
}

// setBaseResume stores src as the canonical base resume under resume/resume.<ext>,
// writes the extracted resume/resume.txt, and records cfg.ResumePath. It returns
// the stored source's relative path and the extracted text length.
func setBaseResume(s *store.Store, src string) (string, int, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", 0, err
	}
	ext := strings.ToLower(filepath.Ext(src))
	rel := filepath.Join("resume", "resume"+ext)
	if err := s.WriteFile(rel, raw); err != nil {
		return "", 0, err
	}
	text, err := extractText(src, ext, raw)
	if err != nil {
		return "", 0, err
	}
	if err := s.WriteFile(filepath.Join("resume", "resume.txt"), []byte(text)); err != nil {
		return "", 0, err
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return "", 0, err
	}
	cfg.ResumePath = rel
	if err := s.SaveConfig(cfg); err != nil {
		return "", 0, err
	}
	return rel, len(text), nil
}

func newResumeAddCmd() *cobra.Command {
	var roleID string
	cmd := &cobra.Command{
		Use:   "add --role <id> <file.md|file.pdf|file.json>",
		Short: "Store a tailored resume variant for a role (one per role, overwrites)",
		Long: "Store a per-role tailored resume under that role's company folder, plus an\n" +
			"extracted .txt sibling. One variant per role: a re-add overwrites it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(roleID) == "" {
				return fmt.Errorf("--role is required")
			}
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			allRoles, err := s.LoadRoles()
			if err != nil {
				return err
			}
			r, ok := roles.Find(allRoles, roleID)
			if !ok {
				return fmt.Errorf("no role matching id %q", roleID)
			}

			src := args[0]
			raw, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			ext := strings.ToLower(filepath.Ext(src))
			slug := roleCompanySlug(r)
			srcRel := tailoredResumePath(slug, r.GlobalID, ext)
			if err := s.WriteFile(srcRel, raw); err != nil {
				return err
			}
			text, err := extractText(src, ext, raw)
			if err != nil {
				return err
			}
			txtRel := tailoredResumeTextPath(slug, r.GlobalID)
			if err := s.WriteFile(txtRel, []byte(text)); err != nil {
				return err
			}

			if wantJSON(cmd) {
				return emitJSON(map[string]any{
					"role":        r.GlobalID,
					"company":     slug,
					"resume_path": s.Path(srcRel),
					"text_path":   s.Path(txtRel),
					"text_bytes":  len(text),
				})
			}
			info("Stored tailored resume for %s (%s) at %s (%d bytes)", r.GlobalID, r.Title, s.Path(srcRel), len(text))
			return nil
		},
	}
	cmd.Flags().StringVar(&roleID, "role", "", "role id to tailor for (full global_id or unambiguous prefix)")
	return cmd
}

func newResumeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id|base>",
		Short: "Print a resume's extracted text (base, or a role's tailored variant)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			path, label, err := resolveResumeText(s, args[0])
			if err != nil {
				return err
			}
			text, found, err := readResumeText(path)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no extracted text for %s; nothing stored yet", label)
			}
			fmt.Print(text)
			if !strings.HasSuffix(text, "\n") {
				fmt.Println()
			}
			return nil
		},
	}
}

func newResumeDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <id> | <idA> <idB>",
		Short: "Unified diff of extracted resume text (base vs a role, or two roles)",
		Long: "With one role id, diff the base resume against that role's tailored variant.\n" +
			"With two role ids, diff the two variants. Computed on demand from the current\n" +
			".txt files; jl stores no resume versions.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			var aArg, bArg string
			if len(args) == 1 {
				aArg, bArg = "base", args[0]
			} else {
				aArg, bArg = args[0], args[1]
			}

			aPath, aLabel, err := resolveResumeText(s, aArg)
			if err != nil {
				return err
			}
			bPath, bLabel, err := resolveResumeText(s, bArg)
			if err != nil {
				return err
			}
			aText, aFound, err := readResumeText(aPath)
			if err != nil {
				return err
			}
			if !aFound {
				return fmt.Errorf("no extracted text for %s", aLabel)
			}
			bText, bFound, err := readResumeText(bPath)
			if err != nil {
				return err
			}
			if !bFound {
				return fmt.Errorf("no extracted text for %s", bLabel)
			}

			out := unifiedDiff(aLabel, bLabel, aText, bText)
			if out == "" {
				info("no differences between %s and %s", aLabel, bLabel)
				return nil
			}
			fmt.Print(out)
			return nil
		},
	}
}

func newResumeRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Remove a tailored resume variant (the base is replaced via resume set)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.EqualFold(args[0], "base") {
				return fmt.Errorf("the base resume is not removable via rm; replace it with: jl resume set <file>")
			}
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			allRoles, err := s.LoadRoles()
			if err != nil {
				return err
			}
			slug := ""
			id := args[0]
			if r, ok := roles.Find(allRoles, id); ok {
				slug = roleCompanySlug(r)
				id = r.GlobalID
			}
			// Without a matching role we can still try to remove by the stored
			// variant whose role id matches the argument exactly.
			if slug == "" {
				for _, v := range listTailoredResumes(s) {
					if v.role == id {
						slug = v.slug
						break
					}
				}
			}
			if slug == "" {
				return fmt.Errorf("no tailored resume found for %q", args[0])
			}
			srcRemoved := removeGlob(s, tailoredResumeGlob(slug, id))
			txtRemoved := removeIfExists(s, tailoredResumeTextPath(slug, id))
			if !srcRemoved && !txtRemoved {
				return fmt.Errorf("no tailored resume found for %q", args[0])
			}
			info("Removed tailored resume for %s", id)
			return nil
		},
	}
}

// --- tailored-variant path helpers ---

// roleCompanySlug derives the canonical company slug for a role, falling back to
// the slug of its display employer when the role carries no company label.
func roleCompanySlug(r model.Role) string {
	if r.Company != "" {
		return r.Company
	}
	return model.Slug(r.Employer)
}

// findRoleBySanitizedID returns the role whose sanitized GlobalID equals frag,
// so a stored tailored variant (named by the sanitized id) can be mapped back to
// its real role for display and lookup.
func findRoleBySanitizedID(all []model.Role, frag string) (model.Role, bool) {
	for _, r := range all {
		if sanitizeRoleID(r.GlobalID) == frag {
			return r, true
		}
	}
	return model.Role{}, false
}

// sanitizeRoleID makes a role id safe to use as a filename fragment: ':' and any
// other path-unsafe characters collapse to '-'. It is deterministic so ls, show,
// diff, rm, and log --from-role all derive the same tailored path from an id.
func sanitizeRoleID(id string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range id {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == '.' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// tailoredResumePath is the canonical source path for a role's tailored resume,
// relative to the data dir: companies/<slug>/resume-<sanitized-id>.<ext>.
func tailoredResumePath(slug, roleID, ext string) string {
	return filepath.Join("companies", slug, "resume-"+sanitizeRoleID(roleID)+ext)
}

// tailoredResumeTextPath is the extracted-text sibling of tailoredResumePath.
func tailoredResumeTextPath(slug, roleID string) string {
	return filepath.Join("companies", slug, "resume-"+sanitizeRoleID(roleID)+".txt")
}

// tailoredResumeGlob matches the source file(s) for a role's tailored resume,
// across whatever extension it was stored with (but not the .txt sibling).
func tailoredResumeGlob(slug, roleID string) string {
	return filepath.Join("companies", slug, "resume-"+sanitizeRoleID(roleID)+".*")
}

// tailoredResume is one stored tailored variant discovered on disk.
type tailoredResume struct {
	role   string // the sanitized id fragment used in the filename
	slug   string
	source string // relative path to the canonical source
	text   string // relative path to the extracted .txt
}

// listTailoredResumes scans every company folder for tailored resume sources
// (resume-<id>.<ext>, excluding the .txt extraction) and returns them sorted.
// The "role" it reports is the sanitized filename fragment, which round-trips
// for lookup because sanitizeRoleID is idempotent on already-safe ids.
func listTailoredResumes(s *store.Store) []tailoredResume {
	companiesDir := s.Path("companies")
	slugs, err := os.ReadDir(companiesDir)
	if err != nil {
		return nil
	}
	var out []tailoredResume
	for _, sd := range slugs {
		if !sd.IsDir() {
			continue
		}
		slug := sd.Name()
		files, err := os.ReadDir(filepath.Join(companiesDir, slug))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasPrefix(name, "resume-") {
				continue
			}
			ext := filepath.Ext(name)
			if ext == ".txt" {
				continue
			}
			id := strings.TrimSuffix(strings.TrimPrefix(name, "resume-"), ext)
			out = append(out, tailoredResume{
				role:   id,
				slug:   slug,
				source: filepath.Join("companies", slug, name),
				text:   filepath.Join("companies", slug, "resume-"+id+".txt"),
			})
		}
	}
	return out
}

// resolveResumeText maps a show/diff selector ("base" or a role id) to the path
// of its extracted .txt and a human label. A role id is resolved against the
// index when possible, else treated as an already-sanitized filename fragment.
func resolveResumeText(s *store.Store, sel string) (path, label string, err error) {
	if strings.EqualFold(sel, "base") {
		return s.Path("resume", "resume.txt"), "base", nil
	}
	allRoles, err := s.LoadRoles()
	if err != nil {
		return "", "", err
	}
	if r, ok := roles.Find(allRoles, sel); ok {
		return s.Path(tailoredResumeTextPath(roleCompanySlug(r), r.GlobalID)), r.GlobalID, nil
	}
	// Fall back to a stored variant whose sanitized id matches.
	for _, v := range listTailoredResumes(s) {
		if v.role == sanitizeRoleID(sel) {
			return s.Path(v.text), sel, nil
		}
	}
	return "", "", fmt.Errorf("no role or tailored resume matching id %q", sel)
}

func removeIfExists(s *store.Store, rel string) bool {
	if err := os.Remove(s.Path(rel)); err == nil {
		return true
	}
	return false
}

func removeGlob(s *store.Store, relGlob string) bool {
	matches, _ := filepath.Glob(s.Path(relGlob))
	removed := false
	for _, m := range matches {
		if filepath.Ext(m) == ".txt" {
			continue
		}
		if err := os.Remove(m); err == nil {
			removed = true
		}
	}
	return removed
}

// extractText returns the plaintext for a resume source: PDFs are extracted via
// pdfToText, everything else passes through.
func extractText(src, ext string, raw []byte) (string, error) {
	if ext == ".pdf" {
		// Prefer poppler's pdftotext when it is on PATH: the pure-Go reader
		// (dslipak/pdf) reports no glyph widths for some PDFs, which collapses
		// every word together. Fall back to it only when pdftotext is absent or
		// yields nothing.
		if text, ok := pdftotextExtract(src); ok {
			return text, nil
		}
		text, err := pdfToText(src)
		if err != nil {
			return "", fmt.Errorf("extract pdf text: %w", err)
		}
		return text, nil
	}
	return string(raw), nil
}

// pdftotextExtract runs poppler's `pdftotext` to read a PDF as plaintext. It
// returns ok=false (so the caller falls back to the pure-Go reader) when the
// binary is not installed, the command fails, or the output is blank.
func pdftotextExtract(src string) (string, bool) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", false
	}
	// `-nopgbrk` drops the form-feed between pages; `-` writes to stdout.
	out, err := exec.Command(bin, "-nopgbrk", src, "-").Output()
	if err != nil {
		return "", false
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", false
	}
	return string(out), true
}

// textFrag is one positioned text fragment on a row: its left edge X, width W,
// and string S. It mirrors the fields of pdf.Text we use, decoupling the
// space-insertion heuristic from the PDF library so it can be unit-tested.
type textFrag struct {
	X, W float64
	S    string
}

// assembleRows joins extracted fragments into lines, inserting a space where a
// horizontal gap separates two fragments on a row. dslipak/pdf's GetPlainText
// concatenates fragments with no spaces for many PDFs (LaTeX output among them),
// so the gap is the only signal of a word boundary.
func assembleRows(rows [][]textFrag) string {
	var b strings.Builder
	for _, row := range rows {
		var prevEnd float64
		for j, t := range row {
			if j > 0 && t.X-prevEnd > 1.0 && !strings.HasSuffix(b.String(), " ") {
				b.WriteByte(' ')
			}
			b.WriteString(t.S)
			prevEnd = t.X + t.W
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

// pdfToText extracts readable plaintext from a PDF using dslipak/pdf (pure Go).
// It walks text row by row and inserts spaces via assembleRows, falling back to
// the library's GetPlainText when row extraction yields nothing.
func pdfToText(path string) (string, error) {
	r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			return "", err
		}
		frags := make([][]textFrag, 0, len(rows))
		for _, row := range rows {
			line := make([]textFrag, 0, len(row.Content))
			for _, t := range row.Content {
				line = append(line, textFrag{X: t.X, W: t.W, S: t.S})
			}
			frags = append(frags, line)
		}
		b.WriteString(assembleRows(frags))
	}
	text := b.String()
	if strings.TrimSpace(text) == "" {
		// Fall back to GetPlainText if row extraction yielded nothing.
		rc, err := r.GetPlainText()
		if err != nil {
			return "", err
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return text, nil
}
