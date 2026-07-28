package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	return collectRows[Project](s.pool.Query(ctx, `SELECT * FROM projects ORDER BY updated_at DESC`))
}

func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (Project, error) {
	return one[Project](s.pool.Query(ctx, `SELECT * FROM projects WHERE id=$1`, id))
}

func (s *Store) CreateProject(ctx context.Context, title, description, aspectRatio, resolution string, frameRate *float64) (Project, error) {
	return one[Project](s.pool.Query(ctx, `
		INSERT INTO projects (id,title,description,aspect_ratio,resolution,frame_rate) VALUES ($1,$2,$3,$4,$5,$6) RETURNING *`,
		uuid.New(), strings.TrimSpace(title), strings.TrimSpace(description), strings.TrimSpace(aspectRatio), strings.TrimSpace(resolution), frameRate))
}

func (s *Store) UpdateProject(ctx context.Context, id uuid.UUID, title, description, aspectRatio, resolution string, frameRate *float64) (Project, error) {
	return one[Project](s.pool.Query(ctx, `
		UPDATE projects SET title=$2,description=$3,aspect_ratio=$4,resolution=$5,frame_rate=$6,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 RETURNING *`,
		id, strings.TrimSpace(title), strings.TrimSpace(description), strings.TrimSpace(aspectRatio), strings.TrimSpace(resolution), frameRate))
}

func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func (s *Store) ListEpisodes(ctx context.Context, projectID uuid.UUID) ([]Episode, error) {
	return collectRows[Episode](s.pool.Query(ctx, `SELECT * FROM episodes WHERE project_id=$1 ORDER BY episode_number`, projectID))
}

func (s *Store) GetEpisode(ctx context.Context, id uuid.UUID) (Episode, error) {
	return one[Episode](s.pool.Query(ctx, `SELECT * FROM episodes WHERE id=$1`, id))
}

type CreateEpisodeInput struct {
	ProjectID             uuid.UUID
	EpisodeNumber         int32
	Title                 string
	Notes                 string
	TargetDurationSeconds *int32
	TargetShotCount       *int32
	ScriptBody            string
}

func (s *Store) CreateEpisode(ctx context.Context, input CreateEpisodeInput) (Episode, error) {
	var created Episode
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		number := input.EpisodeNumber
		if number <= 0 {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(episode_number),0)+1 FROM episodes WHERE project_id=$1`, input.ProjectID).Scan(&number); err != nil {
				return err
			}
		}
		var err error
		created, err = one[Episode](tx.Query(ctx, `
			INSERT INTO episodes (id,project_id,episode_number,title,notes,target_duration_seconds,target_shot_count)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *`, uuid.New(), input.ProjectID, number,
			strings.TrimSpace(input.Title), strings.TrimSpace(input.Notes), input.TargetDurationSeconds, input.TargetShotCount))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO episode_scripts (id,episode_id,version,body,status) VALUES ($1,$2,1,$3,'adopted')`, uuid.New(), created.ID, input.ScriptBody)
		return err
	})
	return created, err
}

func (s *Store) UpdateEpisode(ctx context.Context, episode Episode) (Episode, error) {
	var updated Episode
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[Episode](tx.Query(ctx, `SELECT * FROM episodes WHERE id=$1`, episode.ID))
		if err != nil {
			return err
		}
		var swappedID uuid.UUID
		if episode.EpisodeNumber != current.EpisodeNumber {
			err = tx.QueryRow(ctx, `SELECT id FROM episodes WHERE project_id=$1 AND episode_number=$2`, current.ProjectID, episode.EpisodeNumber).Scan(&swappedID)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil {
				var temporary int32
				if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(episode_number),0)+1 FROM episodes WHERE project_id=$1`, current.ProjectID).Scan(&temporary); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE episodes SET episode_number=$2 WHERE id=$1`, swappedID, temporary); err != nil {
					return err
				}
			}
		}
		updated, err = one[Episode](tx.Query(ctx, `
			UPDATE episodes SET episode_number=$2,title=$3,notes=$4,target_duration_seconds=$5,target_shot_count=$6,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 RETURNING *`,
			episode.ID, episode.EpisodeNumber, strings.TrimSpace(episode.Title), strings.TrimSpace(episode.Notes), episode.TargetDurationSeconds, episode.TargetShotCount))
		if err != nil {
			return err
		}
		if swappedID != uuid.Nil {
			_, err = tx.Exec(ctx, `UPDATE episodes SET episode_number=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, swappedID, current.EpisodeNumber)
		}
		return err
	})
	return updated, err
}

func (s *Store) DeleteEpisode(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM episodes WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func (s *Store) ListEpisodeScripts(ctx context.Context, episodeID uuid.UUID) ([]EpisodeScript, error) {
	return collectRows[EpisodeScript](s.pool.Query(ctx, `SELECT * FROM episode_scripts WHERE episode_id=$1 ORDER BY version DESC`, episodeID))
}

func (s *Store) CreateEpisodeScript(ctx context.Context, episodeID uuid.UUID, body, note string, adopt bool) (EpisodeScript, error) {
	var script EpisodeScript
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		var version int32
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM episode_scripts WHERE episode_id=$1`, episodeID).Scan(&version); err != nil {
			return err
		}
		status := "candidate"
		if adopt {
			status = "adopted"
			if _, err := tx.Exec(ctx, `UPDATE episode_scripts SET status='candidate' WHERE episode_id=$1 AND status='adopted'`, episodeID); err != nil {
				return err
			}
		}
		var err error
		script, err = one[EpisodeScript](tx.Query(ctx, `
			INSERT INTO episode_scripts (id,episode_id,version,body,note,status) VALUES ($1,$2,$3,$4,$5,$6) RETURNING *`,
			uuid.New(), episodeID, version, body, note, status))
		return err
	})
	return script, err
}

func (s *Store) AdoptEpisodeScript(ctx context.Context, id uuid.UUID) (EpisodeScript, error) {
	var script EpisodeScript
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		var episodeID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT episode_id FROM episode_scripts WHERE id=$1`, id).Scan(&episodeID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE episode_scripts SET status='candidate' WHERE episode_id=$1 AND status='adopted'`, episodeID); err != nil {
			return err
		}
		var err error
		script, err = one[EpisodeScript](tx.Query(ctx, `UPDATE episode_scripts SET status='adopted' WHERE id=$1 RETURNING *`, id))
		return err
	})
	return script, err
}

func ValidateEpisode(input CreateEpisodeInput) error {
	if input.ProjectID == uuid.Nil {
		return fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("title is required")
	}
	return nil
}
