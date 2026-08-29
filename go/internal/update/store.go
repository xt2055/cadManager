package update

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Record struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	Mandatory   bool   `json:"mandatory"`
	Enabled     bool   `json:"enabled"`
	FileName    string `json:"fileName,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var ErrNotFound = errors.New("更新版本不存在")

func (store *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, version, platform, notes, published_at, download_url, mandatory, enabled, file_name, size_bytes, created_at
		FROM update_manifests
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Record, 0)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

// Latest 返回启用状态中版本号最高的记录。
func (store *Store) Latest(ctx context.Context) (*Record, error) {
	records, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	var best *Record
	for index := range records {
		record := records[index]
		if !record.Enabled {
			continue
		}
		if best == nil || CompareVersions(record.Version, best.Version) > 0 {
			best = &records[index]
		}
	}
	return best, nil
}

func (store *Store) Get(ctx context.Context, id string) (*Record, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, version, platform, notes, published_at, download_url, mandatory, enabled, file_name, size_bytes, created_at
		FROM update_manifests
		WHERE id = $1
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	record, err := scanRecord(rows)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (store *Store) Create(ctx context.Context, record Record) (Record, error) {
	var published any
	if record.PublishedAt != "" {
		published = record.PublishedAt
	}
	var downloadURL any
	if record.DownloadURL != "" {
		downloadURL = record.DownloadURL
	}
	row := store.pool.QueryRow(ctx, `
		INSERT INTO update_manifests (version, platform, notes, published_at, download_url, mandatory, enabled, file_name, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8)
		RETURNING id
	`, record.Version, record.Platform, record.Notes, published, downloadURL, record.Mandatory, record.FileName, record.SizeBytes)
	var id string
	if err := row.Scan(&id); err != nil {
		return Record{}, err
	}
	record.ID = id
	if record.PublishedAt == "" {
		record.PublishedAt = time.Now().Format("2006-01-02 15:04")
	}
	return record, nil
}

func (store *Store) SetDownloadURL(ctx context.Context, id string, url string) error {
	_, err := store.pool.Exec(ctx, `UPDATE update_manifests SET download_url = $2 WHERE id = $1`, id, url)
	return err
}

func (store *Store) Delete(ctx context.Context, id string) error {
	tag, err := store.pool.Exec(ctx, `DELETE FROM update_manifests WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanRecord(rows interface {
	Scan(dest ...any) error
}) (Record, error) {
	var record Record
	var published *time.Time
	var created *time.Time
	var downloadURL *string
	if err := rows.Scan(&record.ID, &record.Version, &record.Platform, &record.Notes, &published, &downloadURL, &record.Mandatory, &record.Enabled, &record.FileName, &record.SizeBytes, &created); err != nil {
		return record, err
	}
	if published != nil {
		record.PublishedAt = published.Local().Format("2006-01-02 15:04")
	}
	if created != nil {
		record.CreatedAt = created.Local().Format("2006-01-02 15:04")
	}
	if downloadURL != nil {
		record.DownloadURL = *downloadURL
	}
	return record, nil
}
