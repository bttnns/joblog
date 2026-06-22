package cli

import (
	"strings"
	"time"

	"github.com/bttnns/joblog/internal/dates"
	"github.com/bttnns/joblog/internal/store"
)

// nowFunc returns the current time. It is a var so tests can pin it.
var nowFunc = time.Now

// storeNewID is a thin indirection over store.NewID for entry creation.
func storeNewID() string { return store.NewID() }

// joinVocab renders a controlled vocabulary for help and error text.
func joinVocab(v []string) string { return strings.Join(v, ", ") }

// validDate reports whether s is a valid ISO date.
func validDate(s string) bool {
	_, err := time.Parse(dates.ISO, s)
	return err == nil
}
