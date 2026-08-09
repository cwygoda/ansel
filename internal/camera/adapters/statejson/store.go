package statejson

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

type Store struct {
	path    string
	records map[string]domain.ImportRecord
}

type stateFile struct {
	Version int                   `json:"version"`
	Records []domain.ImportRecord `json:"records"`
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, records: make(map[string]domain.ImportRecord)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("failed to read import state: %w", err)
	}
	var sf stateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse import state: %w", err)
	}
	for _, record := range sf.Records {
		store.records[record.Key()] = record
	}
	return store, nil
}

func (s *Store) IsImported(fileKey string) bool {
	_, ok := s.records[fileKey]
	return ok
}

func (s *Store) Record(record domain.ImportRecord) error {
	s.records[record.Key()] = record
	return nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	records := make([]domain.ImportRecord, 0, len(s.records))
	for _, record := range s.records {
		records = append(records, record)
	}
	data, err := json.MarshalIndent(stateFile{Version: 1, Records: records}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal import state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("failed to write import state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("failed to replace import state: %w", err)
	}
	return nil
}
