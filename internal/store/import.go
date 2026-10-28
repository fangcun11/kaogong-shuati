package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

type ImportStats struct {
	Papers       int
	Questions    int
	Materials    int
	Categories   int
	QMaterials   int
	QMaterialMap int
}

// ImportLegacy copies the read-only question bank and materials into the
// new single-file database. It intentionally does not copy legacy user data;
// callers must handle that explicitly.
func ImportLegacy(ctx context.Context, db *sql.DB, tikuPath, materialsPath, indexPath string) (ImportStats, error) {
	var stats ImportStats
	if _, err := os.Stat(tikuPath); err != nil {
		return stats, fmt.Errorf("open tiku.db: %w", err)
	}
	src, err := sql.Open("sqlite", "file:"+tikuPath+"?mode=ro")
	if err != nil {
		return stats, err
	}
	defer src.Close()
	if err := src.PingContext(ctx); err != nil {
		return stats, fmt.Errorf("ping tiku.db: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	if err := copyPapers(ctx, tx, src, &stats); err != nil {
		return stats, fmt.Errorf("import papers: %w", err)
	}
	if err := copyQuestions(ctx, tx, src, &stats); err != nil {
		return stats, fmt.Errorf("import questions: %w", err)
	}

	if materialsPath != "" {
		if _, err := os.Stat(materialsPath); err == nil {
			mdb, err := sql.Open("sqlite", "file:"+materialsPath+"?mode=ro")
			if err != nil {
				return stats, err
			}
			defer mdb.Close()
			if err := copyMaterials(ctx, tx, mdb, &stats); err != nil {
				return stats, fmt.Errorf("import materials: %w", err)
			}
		}
	}

	if indexPath != "" {
		if _, err := os.Stat(indexPath); err == nil {
			idb, err := sql.Open("sqlite", "file:"+indexPath+"?mode=ro")
			if err != nil {
				return stats, err
			}
			defer idb.Close()
			if err := copyIndex(ctx, tx, idb, &stats); err != nil {
				return stats, fmt.Errorf("import index: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

func copyPapers(ctx context.Context, tx *sql.Tx, src *sql.DB, stats *ImportStats) error {
	rows, err := src.QueryContext(ctx, `SELECT id, subject, subjectName, category, name, questionCount, difficulty, chapters FROM papers`)
	if err != nil {
		return err
	}
	defer rows.Close()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO papers (id, subject, subjectName, category, name, questionCount, difficulty, chapters)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for rows.Next() {
		var id int64
		var subject, subjectName, category, name string
		var questionCount sql.NullInt64
		var difficulty sql.NullFloat64
		var chapters sql.NullString
		if err := rows.Scan(&id, &subject, &subjectName, &category, &name, &questionCount, &difficulty, &chapters); err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, id, subject, subjectName, category, name, questionCount, difficulty, chapters); err != nil {
			return err
		}
		stats.Papers++
	}
	return rows.Err()
}

func copyQuestions(ctx context.Context, tx *sql.Tx, src *sql.DB, stats *ImportStats) error {
	rows, err := src.QueryContext(ctx, `
		SELECT id, questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO questions
		(id, questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for rows.Next() {
		var id, questionID, paperID int64
		var chapter, content, contentHTML, options, answer, analysis sql.NullString
		var qtype, answerIndex, difficulty sql.NullInt64
		if err := rows.Scan(&id, &questionID, &paperID, &chapter, &qtype, &content, &contentHTML, &options, &answer, &answerIndex, &difficulty, &analysis); err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, id, questionID, paperID, chapter, qtype, content, contentHTML, options, answer, answerIndex, difficulty, analysis); err != nil {
			return err
		}
		stats.Questions++
	}
	return rows.Err()
}

func copyMaterials(ctx context.Context, tx *sql.Tx, src *sql.DB, stats *ImportStats) error {
	rows, err := src.QueryContext(ctx, `SELECT paperId, title, idx, text FROM materials ORDER BY paperId, idx`)
	if err != nil {
		return err
	}
	defer rows.Close()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO materials (paperId, title, idx, text) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for rows.Next() {
		var paperID, idx int64
		var title, text string
		if err := rows.Scan(&paperID, &title, &idx, &text); err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, paperID, title, idx, text); err != nil {
			return err
		}
		stats.Materials++
	}
	return rows.Err()
}

func copyIndex(ctx context.Context, tx *sql.Tx, src *sql.DB, stats *ImportStats) error {
	type tableCopy struct {
		query string
		stmt  string
		count *int
	}
	copies := []tableCopy{
		{`SELECT question_id, subject, category, sub FROM question_categories`,
			`INSERT OR REPLACE INTO question_categories (question_id, subject, category, sub) VALUES (?, ?, ?, ?)`, &stats.Categories},
		{`SELECT material_id, subject, content FROM q_materials`,
			`INSERT OR REPLACE INTO q_materials (material_id, subject, content) VALUES (?, ?, ?)`, &stats.QMaterials},
		{`SELECT question_id, subject, material_id FROM q_material_map`,
			`INSERT OR REPLACE INTO q_material_map (question_id, subject, material_id) VALUES (?, ?, ?)`, &stats.QMaterialMap},
	}
	for _, c := range copies {
		rows, err := src.QueryContext(ctx, c.query)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, c.stmt)
		if err != nil {
			rows.Close()
			return err
		}
		for rows.Next() {
			var a int64
			var b string
			var d sql.NullString
			if c.count == &stats.QMaterialMap {
				var mid sql.NullInt64
				if err := rows.Scan(&a, &b, &mid); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
				if _, err := stmt.ExecContext(ctx, a, b, mid); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
			} else if c.count == &stats.QMaterials {
				if err := rows.Scan(&a, &b, &d); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
				if _, err := stmt.ExecContext(ctx, a, b, d); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
			} else {
				var sub string
				if err := rows.Scan(&a, &b, &d, &sub); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
				if _, err := stmt.ExecContext(ctx, a, b, d, sub); err != nil {
					rows.Close()
					stmt.Close()
					return err
				}
			}
			(*c.count)++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			stmt.Close()
			return err
		}
		rows.Close()
		stmt.Close()
	}
	return nil
}
