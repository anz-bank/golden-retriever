package pinner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Mod is the struct of the file which stores the dependency requirements
type Mod struct {
	Imports map[string]*Import `yaml:"imports"`
	modFile string
	mutex   sync.RWMutex
}

// Import is the dependency requirement with specified reference and pinned version
type Import struct {
	Ref    string `yaml:"ref,omitempty"`
	Pinned string `yaml:"pinned"`
}

// NewMod initializes and returns a new Mod instance
func NewMod(modFile string) (*Mod, error) {
	m := &Mod{modFile: modFile, mutex: sync.RWMutex{}, Imports: make(map[string]*Import)}
	if _, err := os.Stat(modFile); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(modFile), os.ModePerm)
		if err != nil {
			return nil, err
		}
		f, err := os.Create(modFile)
		if err != nil {
			return nil, err
		}
		f.Close()
		return m, nil
	} else if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadFile(modFile)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(b, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetImport returns Import with given repository key
func (m *Mod) GetImport(repo string) (*Import, bool) {
	m.mutex.RLock()
	im, ok := m.Imports[repo]
	m.mutex.RUnlock()
	return im, ok
}

// GetImport sets value Import with given repository key
func (m *Mod) SetImport(repo string, im *Import) {
	m.mutex.Lock()
	m.Imports[repo] = im
	m.mutex.Unlock()
	return
}

// Save Mod content to modFile
func (m *Mod) Save() error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(m.modFile, b, os.ModePerm)
}
