package api

import (
	"database/sql"
	"net/http"
	"time"
)

type subjectRow struct {
	SubjectName string `json:"subjectName"`
	Papers      int64  `json:"papers"`
	Questions   int64  `json:"questions"`
	Done        int64  `json:"done"`
}

func (s *Server) subjects(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	from, to := yearRange()
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT p.subjectName,
		       COUNT(DISTINCT p.id) AS papers,
		       COUNT(DISTINCT q.questionId) AS questions
		FROM papers p
		LEFT JOIN questions q ON q.paperId = p.id
		WHERE substr(p.name, 1, 4) BETWEEN ? AND ?
		GROUP BY p.subjectName
		ORDER BY p.subjectName`, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题库统计失败")
		return
	}
	defer rows.Close()
	out := make([]subjectRow, 0)
	for rows.Next() {
		var x subjectRow
		if err := rows.Scan(&x.SubjectName, &x.Papers, &x.Questions); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题库统计失败")
			return
		}
		out = append(out, x)
	}
	done := map[string]int64{}
	drows, err := s.db.QueryContext(r.Context(), `
		SELECT p.subjectName, COUNT(DISTINCT CAST(rec.question_id AS INTEGER)) AS done
		FROM practice_records rec
		JOIN questions q ON q.questionId = CAST(rec.question_id AS INTEGER)
		JOIN papers p ON p.id = q.paperId
		WHERE rec.user_id = ? AND rec.is_correct = 1
		  AND substr(p.name, 1, 4) BETWEEN ? AND ?
		GROUP BY p.subjectName`, u.ID, from, to)
	if err == nil {
		defer drows.Close()
		for drows.Next() {
			var name string
			var n int64
			if err := drows.Scan(&name, &n); err == nil {
				done[name] = n
			}
		}
	}
	for i := range out {
		out[i].Done = done[out[i].SubjectName]
		if out[i].SubjectName == "事业编·综应" {
			var n int64
			_ = s.db.QueryRowContext(r.Context(), `
				SELECT COUNT(DISTINCT qc.question_id)
				FROM question_categories qc
				JOIN questions q ON q.questionId = qc.question_id
				JOIN papers p ON p.id = q.paperId
				WHERE qc.subject = '事业编·综应' AND substr(p.name,1,4) BETWEEN ? AND ?`,
				from, to).Scan(&n)
			out[i].Questions = n
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type chapterStat struct {
	Chapter string `json:"chapter"`
	Count   int64  `json:"c"`
	OK      int64  `json:"ok"`
}

type dayStat struct {
	Day   string `json:"d"`
	Count int64  `json:"c"`
}

type statsResponse struct {
	Total     int64         `json:"total"`
	Correct   int64         `json:"correct"`
	Wrong     int64         `json:"wrong"`
	Rate      int64         `json:"rate"`
	ByChapter []chapterStat `json:"byChapter"`
	Last7     []dayStat     `json:"last7"`
	Daily     []dayStat     `json:"daily"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	var total, correct, graded, wrong sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN is_correct IS NOT NULL THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN is_correct = 0 AND archived = 0 THEN 1 ELSE 0 END), 0)
		FROM practice_records WHERE user_id = ?`, u.ID).
		Scan(&total, &correct, &graded, &wrong)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
		return
	}
	resp := statsResponse{
		Total:     total.Int64,
		Correct:   correct.Int64,
		Wrong:     wrong.Int64,
		ByChapter: make([]chapterStat, 0),
		Last7:     make([]dayStat, 0),
		Daily:     make([]dayStat, 0),
	}
	if graded.Int64 > 0 {
		resp.Rate = correct.Int64 * 100 / graded.Int64
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT chapter, COUNT(*) c, COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0) ok
		FROM practice_records WHERE user_id = ?
		GROUP BY chapter ORDER BY c DESC LIMIT 20`, u.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var x chapterStat
			if err := rows.Scan(&x.Chapter, &x.Count, &x.OK); err == nil {
				resp.ByChapter = append(resp.ByChapter, x)
			}
		}
	}
	days, err := s.db.QueryContext(r.Context(), `
		SELECT date(created_at) d, COUNT(*) c
		FROM practice_records
		WHERE user_id = ? AND created_at >= date('now','localtime','-6 days')
		GROUP BY date(created_at) ORDER BY d`, u.ID)
	if err == nil {
		defer days.Close()
		for days.Next() {
			var x dayStat
			if err := days.Scan(&x.Day, &x.Count); err == nil {
				resp.Last7 = append(resp.Last7, x)
				resp.Daily = append(resp.Daily, x)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type customBatch struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Count   int64  `json:"count"`
}

func (s *Server) customBatches(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT b.id, b.name, b.subject, COUNT(q.id)
		FROM custom_batches b
		LEFT JOIN custom_questions q ON q.batch_id = b.id
		WHERE b.user_id = ?
		GROUP BY b.id, b.name, b.subject
		ORDER BY b.id DESC`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取自定义题库失败")
		return
	}
	defer rows.Close()
	batches := make([]customBatch, 0)
	for rows.Next() {
		var b customBatch
		if err := rows.Scan(&b.ID, &b.Name, &b.Subject, &b.Count); err == nil {
			batches = append(batches, b)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": batches})
}

func yearRange() (string, string) {
	to := time.Now().Year()
	return itoa(to - 9), itoa(to)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
