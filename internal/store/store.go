// Package store is the whole-file persistence layer: it resolves the data
// directory, reads and writes the JSON and YAML artifacts atomically, and
// scaffolds a fresh data tree. There is no database; each artifact is a single
// file read and written whole, which keeps the model trivial to reason about.
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bttnns/joblog/internal/assets"
	"github.com/bttnns/joblog/internal/model"
	yaml "go.yaml.in/yaml/v3"
)

// schemaVersion is the on-disk version of the JSON stores (log.json, roles.json).
// It is written into every save and checked on load: a file from a newer jl is
// refused rather than silently misread, and a legacy bare-array file (no envelope)
// is recognized as version 0 and read for back-compat. Bump this when the stored
// shape changes, and add a migration in the loaders.
const schemaVersion = 1

// logFile and rolesFile are the versioned envelopes the JSON stores are written
// in. Older data was a bare JSON array (schema 0); the loaders still read that.
type logFile struct {
	Schema  int           `json:"schema"`
	Entries []model.Entry `json:"entries"`
}

type rolesFile struct {
	Schema int          `json:"schema"`
	Roles  []model.Role `json:"roles"`
}

// Config is the per-user settings file (config.yaml).
type Config struct {
	State      string `yaml:"state"`       // active state code, e.g. "tx"
	Min        int    `yaml:"min"`         // weekly activity minimum; 0 means use the state default
	ResumePath string `yaml:"resume_path"` // path to the canonical resume, relative to the data dir
	Scraper    string `yaml:"scraper"`     // scraper command template; empty means DefaultScraper
}

// DefaultScraper is the scraper command template jl fetch shells out to when the
// scraper config key is unset. {ats} and {slug} are substituted per company. It
// invokes jobhive, the documented default producer; jl itself makes no network
// calls (see DESIGN.md "Division of labor").
const DefaultScraper = "jobhive scrape {ats} {slug} --format json"

// Company is one tracked company in companies.yaml, mapping it to its ATS for
// the scraper. Status is one you set: active (in the fetch rotation) or paused
// (tracked, skipped on fetch). (Formerly "Target"; companies.yaml replaced
// targets.yaml.)
type Company struct {
	Name       string `yaml:"name"`
	ATS        string `yaml:"ats"`
	Slug       string `yaml:"slug"`
	CareersURL string `yaml:"careers-url"`
	Status     string `yaml:"status"` // active | paused; empty reads as active (backfill)
}

// Company status values.
const (
	StatusActive = "active"
	StatusPaused = "paused"
)

type companiesFile struct {
	Companies []Company `yaml:"companies"`
}

// Store is a handle on a resolved data directory.
type Store struct{ Dir string }

// Open resolves the data directory (honoring override) and returns a Store. It
// does not require the directory to exist yet; Init creates it.
func Open(override string) (*Store, error) {
	dir, err := Resolve(override)
	if err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

// Resolve picks the data directory by this precedence: an explicit override, then
// $JOBLOG_HOME, then $XDG_DATA_HOME/joblog when XDG_DATA_HOME is set, then a
// ./private directory if one exists (the in-repo workflow), then the default
// ~/.local/share/joblog. The default keeps a public user's data out of any
// source tree so it cannot be committed by accident.
func Resolve(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if v := os.Getenv("JOBLOG_HOME"); v != "" {
		return filepath.Abs(v)
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "joblog"), nil
	}
	if fi, err := os.Stat("private"); err == nil && fi.IsDir() {
		return filepath.Abs("private")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "joblog"), nil
}

// Path joins parts onto the data directory.
func (s *Store) Path(parts ...string) string {
	return filepath.Join(append([]string{s.Dir}, parts...)...)
}

func (s *Store) logPath() string           { return s.Path("data", "log.json") }
func (s *Store) rolesPath() string         { return s.Path("data", "roles.json") }
func (s *Store) companiesPath() string     { return s.Path("data", "companies.yaml") }
func (s *Store) legacyTargetsPath() string { return s.Path("data", "targets.yaml") }
func (s *Store) configPath() string        { return s.Path("config.yaml") }
func (s *Store) lanesPath() string         { return s.Path("lanes.yaml") }

// Initialized reports whether the data directory has been scaffolded.
func (s *Store) Initialized() bool {
	_, err := os.Stat(s.Path("data"))
	return err == nil
}

// LoadLog reads the work-search log. A missing file is an empty log. Both the
// current versioned envelope and a legacy bare-array file (schema 0) are read.
func (s *Store) LoadLog() ([]model.Entry, error) {
	b, err := os.ReadFile(s.logPath())
	if os.IsNotExist(err) || len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if looksLikeArray(b) { // legacy schema 0: a bare JSON array, no envelope.
		var entries []model.Entry
		if err := json.Unmarshal(b, &entries); err != nil {
			return nil, fmt.Errorf("parse log.json: %w", err)
		}
		return entries, nil
	}
	var lf logFile
	if err := json.Unmarshal(b, &lf); err != nil {
		return nil, fmt.Errorf("parse log.json: %w", err)
	}
	if lf.Schema > schemaVersion {
		return nil, fmt.Errorf("log.json is schema v%d but this jl understands only v%d; upgrade jl", lf.Schema, schemaVersion)
	}
	return lf.Entries, nil
}

// SaveLog writes the work-search log atomically in the current versioned envelope.
func (s *Store) SaveLog(entries []model.Entry) error {
	if entries == nil {
		entries = []model.Entry{}
	}
	return writeJSON(s.logPath(), logFile{Schema: schemaVersion, Entries: entries})
}

// LoadRoles reads the roles index. A missing file is an empty index. Both the
// current versioned envelope and a legacy bare-array file (schema 0) are read.
func (s *Store) LoadRoles() ([]model.Role, error) {
	b, err := os.ReadFile(s.rolesPath())
	if os.IsNotExist(err) || len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if looksLikeArray(b) { // legacy schema 0: a bare JSON array, no envelope.
		var roles []model.Role
		if err := json.Unmarshal(b, &roles); err != nil {
			return nil, fmt.Errorf("parse roles.json: %w", err)
		}
		return roles, nil
	}
	var rf rolesFile
	if err := json.Unmarshal(b, &rf); err != nil {
		return nil, fmt.Errorf("parse roles.json: %w", err)
	}
	if rf.Schema > schemaVersion {
		return nil, fmt.Errorf("roles.json is schema v%d but this jl understands only v%d; upgrade jl", rf.Schema, schemaVersion)
	}
	return rf.Roles, nil
}

// SaveRoles writes the roles index atomically in the current versioned envelope.
func (s *Store) SaveRoles(roles []model.Role) error {
	if roles == nil {
		roles = []model.Role{}
	}
	return writeJSON(s.rolesPath(), rolesFile{Schema: schemaVersion, Roles: roles})
}

// looksLikeArray reports whether b's first non-whitespace byte is '[', i.e. the
// legacy schema-0 form that stored a bare JSON array with no version envelope.
func looksLikeArray(b []byte) bool {
	b = bytes.TrimLeft(b, " \t\r\n")
	return len(b) > 0 && b[0] == '['
}

// LoadCompanies reads companies.yaml. A missing file falls back to the legacy
// targets.yaml (one-time read), so existing data keeps working after the rename;
// the next SaveCompanies writes the new file. A missing legacy file is an empty
// list.
func (s *Store) LoadCompanies() ([]Company, error) {
	b, err := os.ReadFile(s.companiesPath())
	if os.IsNotExist(err) {
		return s.loadLegacyTargets()
	}
	if err != nil {
		return nil, err
	}
	var cf companiesFile
	if err := yaml.Unmarshal(b, &cf); err != nil {
		return nil, fmt.Errorf("parse companies.yaml: %w", err)
	}
	backfillStatus(cf.Companies)
	return cf.Companies, nil
}

// backfillStatus reads any company with an empty Status as active, so data
// written before the status field gains a sensible default without a migration
// step. The next SaveCompanies persists it.
func backfillStatus(companies []Company) {
	for i := range companies {
		if companies[i].Status == "" {
			companies[i].Status = StatusActive
		}
	}
}

// loadLegacyTargets reads the pre-rename targets.yaml (root key "targets"). Its
// fields are identical to Company, so it unmarshals directly.
func (s *Store) loadLegacyTargets() ([]Company, error) {
	b, err := os.ReadFile(s.legacyTargetsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tf struct {
		Targets []Company `yaml:"targets"`
	}
	if err := yaml.Unmarshal(b, &tf); err != nil {
		return nil, fmt.Errorf("parse targets.yaml: %w", err)
	}
	backfillStatus(tf.Targets)
	return tf.Targets, nil
}

// SaveCompanies writes companies.yaml atomically.
func (s *Store) SaveCompanies(companies []Company) error {
	b, err := yaml.Marshal(companiesFile{Companies: companies})
	if err != nil {
		return err
	}
	return writeAtomic(s.companiesPath(), b)
}

// LoadConfig reads config.yaml. A missing file yields zero-value config.
func (s *Store) LoadConfig() (Config, error) {
	var c Config
	b, err := os.ReadFile(s.configPath())
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config.yaml: %w", err)
	}
	return c, nil
}

// LoadLanes reads lanes.yaml from the data dir. If the file is absent it falls
// back to the embedded default so the filter works without a prior `jl init`.
func (s *Store) LoadLanes() (map[string][]string, error) {
	b, err := os.ReadFile(s.lanesPath())
	if os.IsNotExist(err) {
		b = assets.DefaultLanesYAML()
	} else if err != nil {
		return nil, err
	}
	var m map[string][]string
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse lanes.yaml: %w", err)
	}
	// Normalize keys to lowercase so lookups (which lowercase the requested lane)
	// match regardless of how the lane was capitalized in the file.
	normalized := make(map[string][]string, len(m))
	for k, v := range m {
		normalized[strings.ToLower(k)] = v
	}
	return normalized, nil
}

// SaveConfig writes config.yaml atomically.
func (s *Store) SaveConfig(c Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return writeAtomic(s.configPath(), b)
}

// Init scaffolds the data directory. It is idempotent: existing files are never
// overwritten, so it is safe to re-run.
func (s *Store) Init() error {
	dirs := []string{
		s.Path("resume"),
		s.Path("companies"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// Seed empty stores and defaults only when absent. The JSON stores are seeded
	// in the current versioned envelope so a fresh data dir is at the latest schema.
	if !exists(s.logPath()) {
		if err := s.SaveLog(nil); err != nil {
			return err
		}
	}
	if !exists(s.rolesPath()) {
		if err := s.SaveRoles(nil); err != nil {
			return err
		}
	}
	if !exists(s.companiesPath()) {
		if err := s.SaveCompanies(nil); err != nil {
			return err
		}
	}
	if !exists(s.configPath()) {
		if err := s.SaveConfig(Config{}); err != nil {
			return err
		}
	}
	if err := ensureFile(s.Path("README.md"), []byte(assets.DataDirREADME())); err != nil {
		return err
	}
	if err := ensureFile(s.lanesPath(), assets.DefaultLanesYAML()); err != nil {
		return err
	}
	// Profile scaffold, flattened to the data-dir root.
	for rel, content := range assets.ProfileTemplates() {
		p := s.Path(filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := ensureFile(p, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

// Lock takes an exclusive advisory lock on the data directory for the duration
// of a mutating command, so a load-modify-save cycle cannot lose a concurrent
// writer's update (for example a human running `jl add` while an agent runs
// `jl role import` against the same data dir). writeAtomic prevents a torn file;
// this prevents a lost update. The caller defers the returned release function;
// the OS also drops the lock when the process exits. On platforms without
// advisory locking it is a no-op.
func (s *Store) Lock() (release func() error, err error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	return lockFile(filepath.Join(s.Dir, ".lock"))
}

// WriteJSON writes v as JSON to a path relative to the data dir, atomically.
func (s *Store) WriteJSON(rel string, v any) error {
	return writeJSON(s.Path(rel), v)
}

// ReadJSON reads JSON from a path relative to the data dir into v. The bool is
// false when the file does not exist.
func (s *Store) ReadJSON(rel string, v any) (bool, error) {
	p := s.Path(rel)
	if !exists(p) {
		return false, nil
	}
	return true, readJSON(p, v)
}

// WriteFile writes raw bytes to a path relative to the data dir, atomically.
func (s *Store) WriteFile(rel string, data []byte) error {
	return writeAtomic(s.Path(rel), data)
}

// NewID returns a short random hex identifier for an Entry. A failure of the
// system CSPRNG is unrecoverable and would otherwise yield a degenerate
// (all-zero, collision-prone) id, so we panic rather than return a bad id.
func NewID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("store: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// --- file helpers ---

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // leave v at its zero value (empty slice)
	}
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

// writeAtomic writes to a temp file in the same directory and renames it into
// place, so a crash mid-write never corrupts the existing file. The temp file's
// data is fsynced before the rename, and the parent directory is fsynced after,
// so a crash or power loss cannot leave a renamed-but-empty or lost file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Fsync the directory so the rename itself is durable.
	d, err := os.Open(dir)
	if err != nil {
		return nil // rename succeeded; best-effort durability of the dir entry
	}
	defer d.Close()
	_ = d.Sync()
	return nil
}

func ensureFile(path string, data []byte) error {
	if exists(path) {
		return nil
	}
	return writeAtomic(path, data)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
