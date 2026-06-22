// Package assets holds the static templates joblog scaffolds and emits. They are
// embedded so a standalone binary (go install) carries them with no external
// files. This is the single source of truth for these templates.
package assets

import (
	"embed"
)

//go:embed tmpl
var files embed.FS

// ProfileTemplates returns the profile scaffold files keyed by their path
// relative to the data-dir root. The narrative/ folder was flattened, so these
// live at the root now: profile.md (who you are plus what you want next) and
// accomplishments.md (master prose plus remixable STAR beats).
func ProfileTemplates() map[string]string {
	return map[string]string{
		"profile.md":         read("tmpl/profile.md"),
		"accomplishments.md": read("tmpl/accomplishments.md"),
	}
}

// BuildProfilePrompt is the instruction block jl profile init emits on
// stdout (the resume text is appended after it by the caller).
func BuildProfilePrompt() string {
	return read("tmpl/build-profile-prompt.md")
}

// DataDirREADME is the generated README that documents a scaffolded data dir.
func DataDirREADME() string {
	return read("tmpl/datadir-README.md")
}

// DefaultLanesYAML returns the embedded lanes.yaml template. The store writes
// this into a new data dir so the user can edit lane definitions in place.
func DefaultLanesYAML() []byte {
	b, _ := files.ReadFile("tmpl/lanes.yaml")
	return b
}

func read(p string) string {
	b, _ := files.ReadFile(p)
	return string(b)
}
