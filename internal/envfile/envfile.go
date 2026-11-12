// Package envfile reads and edits dotenv files while preserving their
// comments, ordering and formatting. Existing dotenv libraries normalise the
// whole file on write; the EMOBase .env files are documented by hand and are
// read by operators, so surgical edits matter here.
package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type entry struct {
	raw   string
	key   string
	value string
	kv    bool
}

// File is a parsed dotenv file that can be edited in place.
type File struct {
	Path    string
	entries []entry
	dirty   bool
}

// Load parses path. It accepts LF or CRLF and keeps the file's comments and
// blank lines intact.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, string(data)), nil
}

// Parse builds an in-memory file from dotenv content. Edits made to the result
// are written to path by Save.
func Parse(path, content string) *File {
	f := &File{Path: path}
	text := strings.ReplaceAll(content, "\r\n", "\n")
	for _, raw := range strings.Split(text, "\n") {
		f.entries = append(f.entries, parseLine(raw))
	}
	return f
}

func parseLine(raw string) entry {
	e := entry{raw: raw}
	trim := strings.TrimSpace(raw)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return e
	}
	body := trim
	if strings.HasPrefix(body, "export ") {
		body = strings.TrimSpace(strings.TrimPrefix(body, "export "))
	}
	i := strings.Index(body, "=")
	if i <= 0 {
		return e
	}
	key := strings.TrimSpace(body[:i])
	if !validKey(key) {
		return e
	}
	e.key = key
	e.value = parseValue(body[i+1:])
	e.kv = true
	return e
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func parseValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func formatValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, " \t#\"'") {
		escaped := strings.ReplaceAll(v, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return v
}

// Get returns the last value defined for key.
func (f *File) Get(key string) (string, bool) {
	for i := len(f.entries) - 1; i >= 0; i-- {
		if f.entries[i].kv && f.entries[i].key == key {
			return f.entries[i].value, true
		}
	}
	return "", false
}

// Has reports whether key is present and non-empty.
func (f *File) Has(key string) bool {
	v, ok := f.Get(key)
	return ok && v != ""
}

// Keys returns unique keys in file order.
func (f *File) Keys() []string {
	seen := map[string]bool{}
	var keys []string
	for _, e := range f.entries {
		if e.kv && !seen[e.key] {
			seen[e.key] = true
			keys = append(keys, e.key)
		}
	}
	return keys
}

// Set updates an existing key in place, or appends it when absent. It reports
// whether the file changed.
func (f *File) Set(key, value string) bool {
	for i := len(f.entries) - 1; i >= 0; i-- {
		if f.entries[i].kv && f.entries[i].key == key {
			if f.entries[i].value == value && f.entries[i].raw == key+"="+formatValue(value) {
				return false
			}
			f.entries[i].raw = key + "=" + formatValue(value)
			f.entries[i].value = value
			f.dirty = true
			return true
		}
	}
	newEntry := entry{raw: key + "=" + formatValue(value), key: key, value: value, kv: true}
	// Insert before the empty element that represents a trailing newline, so
	// appending does not add an extra blank line.
	if n := len(f.entries); n > 0 && f.entries[n-1].raw == "" {
		f.entries = append(f.entries, entry{})
		copy(f.entries[n:], f.entries[n-1:])
		f.entries[n-1] = newEntry
	} else {
		f.entries = append(f.entries, newEntry)
	}
	f.dirty = true
	return true
}

// Dirty reports whether Save would write anything.
func (f *File) Dirty() bool { return f.dirty }

// Render returns the file contents as they would be saved.
func (f *File) Render() string {
	raw := make([]string, len(f.entries))
	for i, e := range f.entries {
		raw[i] = e.raw
	}
	return strings.Join(raw, "\n")
}

// Save writes the file atomically, preserving its mode. A copy of the previous
// contents is kept in <path>.bak.
func (f *File) Save() error {
	if !f.dirty {
		return nil
	}
	mode := os.FileMode(0o600)
	if st, err := os.Stat(f.Path); err == nil {
		mode = st.Mode().Perm()
		if err := copyFile(f.Path, f.Path+".bak", mode); err != nil {
			return fmt.Errorf("backup %s: %w", f.Path, err)
		}
	}
	dir := filepath.Dir(f.Path)
	tmp, err := os.CreateTemp(dir, ".env.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(f.Render()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, f.Path); err != nil {
		return err
	}
	f.dirty = false
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

// LoadOrEmpty returns the parsed file, or an empty editable file at path when
// it does not exist yet.
func LoadOrEmpty(path string) (*File, error) {
	f, err := Load(path)
	if err == nil {
		return f, nil
	}
	if os.IsNotExist(err) {
		// Represent the trailing newline so appended keys end the file
		// properly without changing how existing files are rendered.
		return &File{Path: path, entries: []entry{{}}}, nil
	}
	return nil, err
}
