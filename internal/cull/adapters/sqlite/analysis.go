package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// CachedAnalysis returns stored observations when nothing that could change
// them has changed.
//
// Reuse is keyed on the content fingerprint and the analyzer set version, and
// deliberately not on policy thresholds: adjusting what counts as "soft"
// recomputes tags, it does not re-measure pixels.
func (s *Store) CachedAnalysis(imageID, fingerprint, analysisVersion string) (domain.Observations, uint64, bool, error) {
	var hash sql.NullInt64
	err := s.db.QueryRow(
		`SELECT perceptual_hash FROM images
		 WHERE id = ? AND fingerprint = ? AND analysis_version = ?`,
		imageID, fingerprint, analysisVersion,
	).Scan(&hash)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to look up cached analysis: %w", err)
	}

	observations, err := s.observationsFor(imageID)
	if err != nil {
		return nil, 0, false, err
	}
	if len(observations) == 0 {
		return nil, 0, false, nil
	}
	return observations, uint64(hash.Int64), true, nil
}

func (s *Store) observationsFor(imageID string) (domain.Observations, error) {
	rows, err := s.db.Query(
		`SELECT o.key, o.value, o.confidence, o.region_id, a.name, a.version
		 FROM observations o
		 JOIN analyzers a ON a.id = o.analyzer_id
		 WHERE o.image_id = ?`, imageID)
	if err != nil {
		return nil, fmt.Errorf("failed to read observations: %w", err)
	}
	defer rows.Close()

	var observations domain.Observations
	for rows.Next() {
		observation, err := scanObservation(rows)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func scanObservation(rows *sql.Rows) (domain.Observation, error) {
	var (
		observation domain.Observation
		confidence  sql.NullFloat64
		regionID    string
	)
	if err := rows.Scan(&observation.Key, &observation.Value, &confidence, &regionID,
		&observation.Analyzer, &observation.Version); err != nil {
		return domain.Observation{}, fmt.Errorf("failed to scan observation: %w", err)
	}
	if confidence.Valid {
		observation.Confidence = &confidence.Float64
	}
	if regionID != "" {
		observation.Region = &domain.Region{ID: regionID}
	}
	return observation, nil
}

// SaveAnalysis records an image and replaces its observations, atomically so a
// failure part-way through cannot leave a half-analyzed photograph that would
// then be treated as cached.
func (s *Store) SaveAnalysis(img domain.Image, analysisVersion string, observations domain.Observations) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := upsertImage(tx, img, analysisVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM observations WHERE image_id = ?`, img.ID); err != nil {
		return fmt.Errorf("failed to clear observations: %w", err)
	}
	if err := insertObservations(tx, img.ID, observations); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertImage(tx *sql.Tx, img domain.Image, analysisVersion string) error {
	_, err := tx.Exec(
		`INSERT INTO images (id, path, file_size, mtime_ns, fingerprint, analysis_version,
		     perceptual_hash, capture_time, width, height, camera, lens,
		     focal_length, aperture, shutter_seconds, iso)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
		     path=excluded.path, file_size=excluded.file_size, mtime_ns=excluded.mtime_ns,
		     fingerprint=excluded.fingerprint, analysis_version=excluded.analysis_version,
		     perceptual_hash=excluded.perceptual_hash, capture_time=excluded.capture_time,
		     width=excluded.width, height=excluded.height, camera=excluded.camera,
		     lens=excluded.lens, focal_length=excluded.focal_length, aperture=excluded.aperture,
		     shutter_seconds=excluded.shutter_seconds, iso=excluded.iso`,
		img.ID, img.Path, img.FileSize, img.MTimeNs, img.Fingerprint, analysisVersion,
		int64(img.PerceptualHash), formatTime(img.Metadata.CaptureTime),
		img.Metadata.Width, img.Metadata.Height, img.Metadata.Camera, img.Metadata.Lens,
		img.Metadata.FocalLength, img.Metadata.Aperture, img.Metadata.ShutterSeconds, img.Metadata.ISO)
	if err != nil {
		return fmt.Errorf("failed to save image %s: %w", img.Path, err)
	}
	return nil
}

func insertObservations(tx *sql.Tx, imageID string, observations domain.Observations) error {
	statement, err := tx.Prepare(
		`INSERT INTO observations (image_id, analyzer_id, key, value, confidence, region_id, created_at)
		 VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare observation insert: %w", err)
	}
	defer statement.Close()

	createdAt := time.Now().UTC().Format(time.RFC3339)
	for _, observation := range observations {
		analyzerID, err := analyzerID(tx, observation.Analyzer, observation.Version)
		if err != nil {
			return err
		}
		if _, err := statement.Exec(imageID, analyzerID, observation.Key, observation.Value,
			confidenceOf(observation), regionOf(observation), createdAt); err != nil {
			return fmt.Errorf("failed to save observation %q: %w", observation.Key, err)
		}
	}
	return nil
}

// analyzerID resolves a name and version to a row, creating it on first sight.
// Recording the analyzer identity is what makes a result reproducible and
// lets a future version invalidate only what it actually changed.
func analyzerID(tx *sql.Tx, name, version string) (int64, error) {
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO analyzers (name, version, model_sha256) VALUES (?,?,'')`,
		name, version); err != nil {
		return 0, fmt.Errorf("failed to register analyzer %s: %w", name, err)
	}

	var id int64
	err := tx.QueryRow(
		`SELECT id FROM analyzers WHERE name = ? AND version = ? AND model_sha256 = ''`,
		name, version).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to look up analyzer %s: %w", name, err)
	}
	return id, nil
}

func confidenceOf(observation domain.Observation) any {
	if observation.Confidence == nil {
		return nil
	}
	return *observation.Confidence
}

func regionOf(observation domain.Observation) string {
	if observation.Region == nil {
		return ""
	}
	return observation.Region.ID
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}
