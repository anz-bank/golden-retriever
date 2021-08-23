package pinner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
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

	if _, err := os.Stat(modFile); err == nil {
		b, err := ioutil.ReadFile(modFile)
		if err != nil {
			return nil, err
		}

		if err := yaml.Unmarshal(b, m); err != nil {
			return nil, err
		}
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
	_, err := os.Stat(m.modFile)

	if os.IsNotExist(err) {
		if len(m.Imports) == 0 {
			return nil
		}

		err = os.MkdirAll(filepath.Dir(m.modFile), os.ModePerm)
		if err != nil {
			return err
		}
		f, err := os.Create(m.modFile)
		if err != nil {
			return err
		}
		defer f.Close()

		log.Debugf("%s created. Pinned versions are saved to this file.\n", m.modFile)
	} else if err != nil {
		return err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(m.modFile, b, os.ModePerm)
}
