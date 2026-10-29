package api

import (
	"database/sql"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	OK    int64  `json:"ok,omitempty"`
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
	where := []string{"user_id = ?"}
	args := []any{u.ID}
	if subject := strings.TrimSpace(r.URL.Query().Get("subject")); subject != "" {
		where = append(where, `question_id IN (
			SELECT q.questionId FROM questions q JOIN papers p ON p.id = q.paperId
			WHERE p.subjectName = ?)`)
		args = append(args, subject)
	}
	if days := queryInt(r, "days", 0, 0, 3650); days > 0 {
		where = append(where, "date(created_at) >= date('now','localtime','-"+strconv.Itoa(days)+" days')")
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		where = append(where, "date(created_at) >= date(?)")
		args = append(args, from)
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		where = append(where, "date(created_at) <= date(?)")
		args = append(args, to)
	}
	whereSQL := strings.Join(where, " AND ")
	var total, correct, graded, wrong sql.NullInt64
	err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN is_correct IS NOT NULL THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN is_correct = 0 AND archived = 0 THEN 1 ELSE 0 END), 0)
		FROM practice_records WHERE `+whereSQL, args...).
		Scan(&total, &correct, &graded, &wrong)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
		return
	}
	resp := statsResponse{
		Total: total.Int64, Correct: correct.Int64, Wrong: wrong.Int64,
		ByChapter: make([]chapterStat, 0), Last7: make([]dayStat, 0), Daily: make([]dayStat, 0),
	}
	if graded.Int64 > 0 {
		resp.Rate = correct.Int64 * 100 / graded.Int64
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT chapter, COUNT(*) c, COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0) ok
		FROM practice_records WHERE `+whereSQL+`
		GROUP BY chapter ORDER BY c DESC LIMIT 20`, args...)
	if err == nil {
		for rows.Next() {
			var x chapterStat
			if rows.Scan(&x.Chapter, &x.Count, &x.OK) == nil {
				resp.ByChapter = append(resp.ByChapter, x)
			}
		}
		rows.Close()
	}
	daysRows, err := s.db.QueryContext(r.Context(), `
		SELECT date(created_at) d, COUNT(*) c,
		       COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0) ok
		FROM practice_records WHERE `+whereSQL+`
		  AND date(created_at) >= date('now','localtime','-7 days')
		GROUP BY date(created_at) ORDER BY d`, args...)
	if err == nil {
		for daysRows.Next() {
			var x dayStat
			if daysRows.Scan(&x.Day, &x.Count, &x.OK) == nil {
				resp.Last7 = append(resp.Last7, x)
			}
		}
		daysRows.Close()
	}
	dailyRows, err := s.db.QueryContext(r.Context(), `
		SELECT date(created_at) d, COUNT(*) c
		FROM practice_records WHERE `+whereSQL+`
		GROUP BY date(created_at) ORDER BY d DESC LIMIT 30`, args...)
	if err == nil {
		for dailyRows.Next() {
			var x dayStat
			if dailyRows.Scan(&x.Day, &x.Count) == nil {
				resp.Daily = append(resp.Daily, x)
			}
		}
		dailyRows.Close()
	}
	writeJSON(w, http.StatusOK, resp)
}

type activitySubject struct {
	Name   string `json:"name"`
	Count  int64  `json:"c"`
	OK     int64  `json:"ok"`
	Graded int64  `json:"graded"`
	CostMs int64  `json:"ms"`
}

type activityDay struct {
	Day      string            `json:"d"`
	Count    int64             `json:"c"`
	OK       int64             `json:"ok"`
	Graded   int64             `json:"graded"`
	CostMs   int64             `json:"ms"`
	Subjects []activitySubject `json:"subjects"`
}

type activityResponse struct {
	From          string            `json:"from"`
	To            string            `json:"to"`
	StartedOn     string            `json:"startedOn"`
	ActiveDays    int64             `json:"activeDays"`
	AllActiveDays int64             `json:"allActiveDays"`
	CurrentStreak int64             `json:"currentStreak"`
	LongestStreak int64             `json:"longestStreak"`
	Total         int64             `json:"total"`
	Correct       int64             `json:"correct"`
	Graded        int64             `json:"graded"`
	CostMs        int64             `json:"ms"`
	Days          []activityDay     `json:"days"`
	Subjects      []activitySubject `json:"subjects"`
}

// activity serves the contribution wall: one entry per day (with a subject
// breakdown) inside a rolling window, plus streak facts that look at the whole
// history so "longest streak" is not clipped by the window.
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	days := queryInt(r, "days", 371, 7, 1825)
	now := time.Now()
	from := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	to := now.Format("2006-01-02")

	dates, startOn, longest, err := s.activityDates(r, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
		return
	}
	current := currentStreak(dates, now)

	resp := activityResponse{
		From: from, To: to, StartedOn: startOn,
		CurrentStreak: current, LongestStreak: longest,
		AllActiveDays: int64(len(dates)),
		Days:          make([]activityDay, 0), Subjects: make([]activitySubject, 0),
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT date(created_at) d, subject,
		       COUNT(*) c,
		       COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0) ok,
		       COALESCE(SUM(CASE WHEN is_correct IS NOT NULL THEN 1 ELSE 0 END), 0) graded,
		       COALESCE(SUM(cost_ms), 0) ms
		FROM practice_records
		WHERE user_id = ? AND date(created_at) >= date(?)
		GROUP BY d, subject
		ORDER BY d, c DESC`, u.ID, from)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
		return
	}
	defer rows.Close()
	dayIndex := map[string]int{}
	subjectIndex := map[string]int{}
	for rows.Next() {
		var d, subject string
		var c, ok, graded, ms int64
		if err := rows.Scan(&d, &subject, &c, &ok, &graded, &ms); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
			return
		}
		di, seen := dayIndex[d]
		if !seen {
			di = len(resp.Days)
			dayIndex[d] = di
			resp.Days = append(resp.Days, activityDay{Day: d, Subjects: make([]activitySubject, 0)})
		}
		day := &resp.Days[di]
		day.Count += c
		day.OK += ok
		day.Graded += graded
		day.CostMs += ms
		if subject != "" {
			day.Subjects = append(day.Subjects, activitySubject{Name: subject, Count: c, OK: ok, Graded: graded, CostMs: ms})

			si, seen := subjectIndex[subject]
			if !seen {
				si = len(resp.Subjects)
				subjectIndex[subject] = si
				resp.Subjects = append(resp.Subjects, activitySubject{Name: subject})
			}
			resp.Subjects[si].Count += c
			resp.Subjects[si].OK += ok
			resp.Subjects[si].Graded += graded
			resp.Subjects[si].CostMs += ms
		}

		resp.Total += c
		resp.Correct += ok
		resp.Graded += graded
		resp.CostMs += ms
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取统计失败")
		return
	}
	resp.ActiveDays = int64(len(resp.Days))
	sort.SliceStable(resp.Subjects, func(i, j int) bool {
		return resp.Subjects[i].Count > resp.Subjects[j].Count
	})
	for i := range resp.Days {
		sort.SliceStable(resp.Days[i].Subjects, func(a, b int) bool {
			return resp.Days[i].Subjects[a].Count > resp.Days[i].Subjects[b].Count
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) activityDates(r *http.Request, userID int64) (map[string]bool, string, int64, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT date(created_at) d FROM practice_records
		WHERE user_id = ? GROUP BY d ORDER BY d`, userID)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()
	dates := map[string]bool{}
	started := ""
	var longest, run int64
	var prev time.Time
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, "", 0, err
		}
		d, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			continue
		}
		dates[raw] = true
		if started == "" {
			started = raw
		}
		if !prev.IsZero() && d.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev = d
	}
	return dates, started, longest, rows.Err()
}

func currentStreak(dates map[string]bool, now time.Time) int64 {
	if len(dates) == 0 {
		return 0
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !dates[day.Format("2006-01-02")] {
		day = day.AddDate(0, 0, -1)
	}
	var streak int64
	for dates[day.Format("2006-01-02")] {
		streak++
		day = day.AddDate(0, 0, -1)
	}
	return streak
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
