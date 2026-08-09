package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// SaveGrouping replaces the groups and ranks for the photographs in this run.
//
// Grouping is derived, not accumulated: re-running with a different similarity
// threshold must produce the new answer, not the union of both. So the
// previous membership for these images is cleared first.
func (s *Store) SaveGrouping(groups []domain.SimilarityGroup, ranks map[string]domain.RankResult) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := clearMembership(tx, groups); err != nil {
		return err
	}
	for _, group := range groups {
		if err := insertGroup(tx, group, ranks); err != nil {
			return err
		}
	}
	if err := dropOrphanGroups(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func clearMembership(tx *sql.Tx, groups []domain.SimilarityGroup) error {
	for _, group := range groups {
		for _, imageID := range group.Members {
			if _, err := tx.Exec(`DELETE FROM group_members WHERE image_id = ?`, imageID); err != nil {
				return fmt.Errorf("failed to clear group membership: %w", err)
			}
		}
	}
	return nil
}

func insertGroup(tx *sql.Tx, group domain.SimilarityGroup, ranks map[string]domain.RankResult) error {
	_, err := tx.Exec(
		`INSERT INTO "groups" (id, kind, representative) VALUES (?,?,?)
		 ON CONFLICT(id) DO UPDATE SET kind=excluded.kind, representative=excluded.representative`,
		group.ID, string(group.Kind), group.Representative)
	if err != nil {
		return fmt.Errorf("failed to save group %s: %w", group.ID, err)
	}

	for _, imageID := range group.Members {
		rank := ranks[imageID]
		if _, err := tx.Exec(
			`INSERT INTO group_members (group_id, image_id, similarity, rank_score, rank_position)
			 VALUES (?,?,?,?,?)`,
			group.ID, imageID, group.Similarities[imageID], rank.Score, rank.Position); err != nil {
			return fmt.Errorf("failed to save group member: %w", err)
		}
	}
	return nil
}

// dropOrphanGroups removes groups left with no members after a regrouping,
// so the table reflects the current answer rather than every answer ever given.
func dropOrphanGroups(tx *sql.Tx) error {
	_, err := tx.Exec(
		`DELETE FROM "groups"
		 WHERE id NOT IN (SELECT DISTINCT group_id FROM group_members)`)
	if err != nil {
		return fmt.Errorf("failed to remove empty groups: %w", err)
	}
	return nil
}

// SaveTags replaces the tags contributed by one source.
//
// Only that source's rows are removed, so a label applied by hand or by
// another tool survives a re-run. Clearing happens even for an image whose
// tag list is now empty, which is why the source is passed in rather than
// inferred from the tags themselves.
func (s *Store) SaveTags(source string, tags map[string][]domain.Tag) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for imageID, imageTags := range tags {
		if err := replaceTags(tx, imageID, source, imageTags); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func replaceTags(tx *sql.Tx, imageID, source string, tags []domain.Tag) error {
	if _, err := tx.Exec(`DELETE FROM tags WHERE image_id = ? AND source = ?`,
		imageID, source); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	for _, tag := range tags {
		if _, err := tx.Exec(
			`INSERT INTO tags (image_id, tag, source, value) VALUES (?,?,?,?)
			 ON CONFLICT(image_id, tag, source) DO UPDATE SET value=excluded.value`,
			imageID, tag.Name, tag.Source, tag.Value); err != nil {
			return fmt.Errorf("failed to save tag %q: %w", tag.Name, err)
		}
	}
	return nil
}
