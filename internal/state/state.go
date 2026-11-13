package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Version is the current state-file schema.
const Version = 1

// Database is one MySQL database created by EMOBase.
type Database struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// Realm is one Keycloak realm created by EMOBase.
type Realm struct {
	Name    string `json:"name"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Created bool   `json:"created"`
}

// Stack is one checkout's ownership record.
type Stack struct {
	Role           string    `json:"role"`
	Name           string    `json:"name"`
	Dir            string    `json:"dir"`
	ComposeProject string    `json:"compose_project,omitempty"`
	Status         string    `json:"status"` // scaffolded, active, pending_cleanup
	Database       *Database `json:"database,omitempty"`
	Realm          *Realm    `json:"realm,omitempty"`
}

// File is the workspace ownership ledger.
type File struct {
	Version int               `json:"version"`
	Stacks  map[string]*Stack `json:"stacks"`
}

// Path returns the state file location for a workspace root.
func Path(root string) string {
	return filepath.Join(root, ".emobase", "state.json")
}

// Load reads the workspace state. A missing file is an empty ledger.
func Load(root string) (*File, error) {
	path := Path(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Version: Version, Stacks: map[string]*Stack{}}, nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.Version == 0 {
		f.Version = Version
	}
	if f.Stacks == nil {
		f.Stacks = map[string]*Stack{}
	}
	return &f, nil
}

// Save writes the ledger atomically.
func (f *File) Save(root string) error {
	if f.Version == 0 {
		f.Version = Version
	}
	if f.Stacks == nil {
		f.Stacks = map[string]*Stack{}
	}
	dir := filepath.Join(root, ".emobase")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, Path(root))
}

// Entry returns the stack entry, creating it when absent.
func (f *File) Entry(key string) *Stack {
	if f.Stacks == nil {
		f.Stacks = map[string]*Stack{}
	}
	s, ok := f.Stacks[key]
	if !ok {
		s = &Stack{}
		f.Stacks[key] = s
	}
	return s
}

// Delete removes a stack entry.
func (f *File) Delete(key string) {
	delete(f.Stacks, key)
}
