package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type checkResult struct {
	OK          *bool    `json:"ok"`
	Correct     []int    `json:"correct"`
	Selected    []int    `json:"selected"`
	CorrectText []string `json:"correctText"`
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuestionID any             `json:"questionId"`
		Selected   json.RawMessage `json:"selected"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	id := stringifyID(req.QuestionID)
	selected := parseSelected(req.Selected)
	if strings.HasPrefix(id, "custom-") {
		q, err := s.findCustomQuestion(r, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
			return
		}
		writeJSON(w, http.StatusOK, gradeCustom(q, selected))
		return
	}
	q, err := s.findQuestion(r, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
		return
	}
	result := gradeQuestion(q, selected)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) submitRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuestionID any             `json:"questionId"`
		Subject    string          `json:"subject"`
		Chapter    string          `json:"chapter"`
		Type       int64           `json:"type"`
		Selected   json.RawMessage `json:"selected"`
		CostMs     int64           `json:"costMs"`
		PaperID    int64           `json:"paperId"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	id := stringifyID(req.QuestionID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 questionId")
		return
	}
	var q *questionJSON
	selected := parseSelected(req.Selected)
	var correct any
	if strings.HasPrefix(id, "custom-") {
		cq, err := s.findCustomQuestion(r, id)
		if err == nil {
			res := gradeCustom(cq, selected)
			if res.OK != nil {
				if *res.OK {
					correct = 1
				} else {
					correct = 0
				}
			}
		}
		if req.Subject == "" {
			req.Subject = "自定义"
		}
	} else {
		q, _ = s.findQuestion(r, id)
		if q != nil {
			res := gradeQuestion(q, selected)
			if res.OK != nil {
				if *res.OK {
					correct = 1
				} else {
					correct = 0
				}
			}
			if req.Subject == "" {
				req.Subject = subjectOfPaper(r, s, q.PaperID)
			}
			if req.Type == 0 {
				req.Type = q.Type
			}
		}
	}
	groupKey := groupForSubject(req.Subject)
	encoded, _ := json.Marshal(selected)
	res, err := s.db.ExecContext(r.Context(), `
		INSERT INTO practice_records
		(user_id, question_id, paper_id, subject, chapter, question_type, selected, is_correct, cost_ms, group_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		currentUserID(r), id, nullInt(req.PaperID), req.Subject, req.Chapter, req.Type, string(encoded), correct, req.CostMs, groupKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "保存记录失败")
		return
	}
	lastID, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": lastID, "removedFromWrong": false})
}

func (s *Server) recent(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	limit := queryInt(r, "limit", 30, 1, 200)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT r.id, r.question_id, r.subject, r.chapter, r.created_at, COALESCE(q.content,''), COALESCE(q.type,0)
		FROM practice_records r
		LEFT JOIN questions q ON q.questionId = CAST(r.question_id AS INTEGER)
		  AND q.id = (SELECT MIN(q2.id) FROM questions q2 WHERE q2.questionId = q.questionId)
		WHERE r.user_id = ? ORDER BY r.id DESC LIMIT ?`, u.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取记录失败")
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, qtype int64
		var qid, subject, chapter, created, content string
		if err := rows.Scan(&id, &qid, &subject, &chapter, &created, &content, &qtype); err == nil {
			out = append(out, map[string]any{"id": id, "questionId": numericID(qid), "subject": subject, "chapter": chapter, "time": created, "content": content, "type": qtype})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) wrongGroups(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT group_key, COUNT(*) FROM practice_records
		WHERE user_id = ? AND is_correct = 0 AND archived = 0 GROUP BY group_key`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取错题分组失败")
		return
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var key string
		var n int64
		if err := rows.Scan(&key, &n); err == nil {
			counts[key] = n
		}
	}
	writeJSON(w, http.StatusOK, sourceGroups(counts))
}

func (s *Server) wrongList(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1<<30)
	group := r.URL.Query().Get("group")
	args := []any{u.ID}
	cond := ""
	if r.URL.Query().Has("group") {
		cond = " AND r.group_key = ?"
		args = append(args, group)
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT r.id, r.question_id, r.subject, r.chapter, r.created_at, COALESCE(q.content,''), COALESCE(q.type,0)
		FROM practice_records r
		LEFT JOIN questions q ON q.questionId = CAST(r.question_id AS INTEGER)
		  AND q.id = (SELECT MIN(q2.id) FROM questions q2 WHERE q2.questionId = q.questionId)
		WHERE r.user_id = ? AND r.is_correct = 0 AND r.archived = 0`+cond+`
		AND r.id IN (SELECT MAX(id) FROM practice_records WHERE user_id = ? AND is_correct = 0 AND archived = 0 GROUP BY question_id)
		ORDER BY r.id DESC LIMIT ? OFFSET ?`, append([]any{u.ID}, args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取错题失败")
		return
	}
	defer rows.Close()
	list := make([]map[string]any, 0)
	for rows.Next() {
		var id, qtype int64
		var qid, subject, chapter, created, content string
		if err := rows.Scan(&id, &qid, &subject, &chapter, &created, &content, &qtype); err == nil {
			list = append(list, map[string]any{"id": id, "questionId": numericID(qid), "subject": subject, "chapter": chapter, "time": created, "content": content, "type": qtype})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": list, "total": len(list), "offset": offset, "limit": limit, "hasMore": false})
}

func (s *Server) removeWrong(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID         int64  `json:"id"`
		QuestionID any    `json:"questionId"`
		Group      string `json:"group"`
	}
	_ = decodeBody(w, r, &req)
	u, _ := userFromContext(r.Context())
	if req.QuestionID != nil {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE practice_records SET archived = 1 WHERE user_id = ? AND question_id = ? AND is_correct = 0`, u.ID, stringifyID(req.QuestionID))
	} else if req.Group != "" {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE practice_records SET archived = 1 WHERE user_id = ? AND group_key = ? AND is_correct = 0`, u.ID, req.Group)
	} else {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE practice_records SET archived = 1 WHERE user_id = ? AND is_correct = 0`, u.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type favoriteItem struct {
	QuestionID any    `json:"questionId"`
	Subject    string `json:"subject"`
	Chapter    string `json:"chapter"`
	Time       string `json:"time"`
	Content    string `json:"content"`
	Type       int64  `json:"type"`
}

func (s *Server) favorites(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		var req struct {
			QuestionID any    `json:"questionId"`
			Subject    string `json:"subject"`
			Chapter    string `json:"chapter"`
		}
		if err := decodeBody(w, r, &req); err != nil || req.QuestionID == nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 questionId")
			return
		}
		id := stringifyID(req.QuestionID)
		if r.Method == http.MethodDelete {
			_, err := s.db.ExecContext(r.Context(), `DELETE FROM favorites WHERE user_id = ? AND question_id = ?`, u.ID, id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "取消收藏失败")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		group := groupForSubject(req.Subject)
		_, err := s.db.ExecContext(r.Context(), `
			INSERT OR IGNORE INTO favorites (user_id, question_id, subject, chapter, group_key)
			VALUES (?, ?, ?, ?, ?)`, u.ID, id, req.Subject, req.Chapter, group)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "收藏失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1<<30)
	args := []any{u.ID}
	cond := ""
	if r.URL.Query().Has("group") {
		cond = " AND f.group_key = ?"
		args = append(args, r.URL.Query().Get("group"))
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT f.question_id, f.subject, f.chapter, f.created_at, COALESCE(q.content,''), COALESCE(q.type,0)
		FROM favorites f
		LEFT JOIN questions q ON q.questionId = CAST(f.question_id AS INTEGER)
		  AND q.id = (SELECT MIN(q2.id) FROM questions q2 WHERE q2.questionId = q.questionId)
		WHERE f.user_id = ?`+cond+` ORDER BY f.rowid DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取收藏失败")
		return
	}
	defer rows.Close()
	list := make([]favoriteItem, 0)
	for rows.Next() {
		var qid, subject, chapter, created, content string
		var qtype int64
		if err := rows.Scan(&qid, &subject, &chapter, &created, &content, &qtype); err == nil {
			list = append(list, favoriteItem{QuestionID: numericID(qid), Subject: subject, Chapter: chapter, Time: created, Content: content, Type: qtype})
		}
	}
	total := countRows(r, s, `SELECT COUNT(*) FROM favorites WHERE user_id = ?`, u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"list": list, "total": total, "offset": offset, "limit": limit, "hasMore": int64(offset+len(list)) < total})
}

func (s *Server) favoriteGroups(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `SELECT group_key, COUNT(*) FROM favorites WHERE user_id = ? GROUP BY group_key`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取收藏分组失败")
		return
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var key string
		var n int64
		if err := rows.Scan(&key, &n); err == nil {
			counts[key] = n
		}
	}
	writeJSON(w, http.StatusOK, sourceGroups(counts))
}

type noteItem struct {
	QuestionID any    `json:"questionId"`
	Subject    string `json:"subject"`
	Chapter    string `json:"chapter"`
	Note       string `json:"note"`
	Time       string `json:"time"`
	Content    string `json:"content"`
	Type       int64  `json:"type"`
}

func (s *Server) notes(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		var req struct {
			QuestionID any    `json:"questionId"`
			Subject    string `json:"subject"`
			Chapter    string `json:"chapter"`
			Note       string `json:"note"`
		}
		if err := decodeBody(w, r, &req); err != nil || req.QuestionID == nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 questionId")
			return
		}
		id := stringifyID(req.QuestionID)
		if r.Method == http.MethodDelete {
			_, _ = s.db.ExecContext(r.Context(), `DELETE FROM notes WHERE user_id = ? AND question_id = ?`, u.ID, id)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		text := strings.TrimSpace(req.Note)
		if text == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "笔记内容不能为空")
			return
		}
		if len([]rune(text)) > 500 {
			text = string([]rune(text)[:500])
		}
		_, err := s.db.ExecContext(r.Context(), `
			INSERT INTO notes (user_id, question_id, subject, chapter, note, group_key)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(user_id, question_id) DO UPDATE SET
			  note = excluded.note, subject = excluded.subject, chapter = excluded.chapter,
			  updated_at = datetime('now','localtime')`,
			u.ID, id, req.Subject, req.Chapter, text, groupForSubject(req.Subject))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "保存笔记失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	qid := r.URL.Query().Get("qid")
	if qid != "" {
		var note, subject, chapter, updated string
		err := s.db.QueryRowContext(r.Context(), `
			SELECT note, subject, chapter, updated_at FROM notes WHERE user_id = ? AND question_id = ?`, u.ID, qid).
			Scan(&note, &subject, &chapter, &updated)
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]any{"list": []any{}, "total": 0, "offset": 0, "limit": 1, "hasMore": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"list": []noteItem{{QuestionID: numericID(qid), Subject: subject, Chapter: chapter, Note: note, Time: updated}}, "total": 1, "offset": 0, "limit": 1, "hasMore": false})
		return
	}
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1<<30)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT n.question_id, n.subject, n.chapter, n.note, n.updated_at, COALESCE(q.content,''), COALESCE(q.type,0)
		FROM notes n
		LEFT JOIN questions q ON q.questionId = CAST(n.question_id AS INTEGER)
		  AND q.id = (SELECT MIN(q2.id) FROM questions q2 WHERE q2.questionId = q.questionId)
		WHERE n.user_id = ? ORDER BY n.updated_at DESC LIMIT ? OFFSET ?`, u.ID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取笔记失败")
		return
	}
	defer rows.Close()
	list := make([]noteItem, 0)
	for rows.Next() {
		var qid, subject, chapter, note, updated, content string
		var qtype int64
		if err := rows.Scan(&qid, &subject, &chapter, &note, &updated, &content, &qtype); err == nil {
			list = append(list, noteItem{QuestionID: numericID(qid), Subject: subject, Chapter: chapter, Note: note, Time: updated, Content: content, Type: qtype})
		}
	}
	total := countRows(r, s, `SELECT COUNT(*) FROM notes WHERE user_id = ?`, u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"list": list, "total": total, "offset": offset, "limit": limit, "hasMore": int64(offset+len(list)) < total})
}

func (s *Server) noteGroups(w http.ResponseWriter, r *http.Request) {
	u, _ := userFromContext(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `SELECT group_key, COUNT(*) FROM notes WHERE user_id = ? GROUP BY group_key`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取笔记分组失败")
		return
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var key string
		var n int64
		if err := rows.Scan(&key, &n); err == nil {
			counts[key] = n
		}
	}
	writeJSON(w, http.StatusOK, sourceGroups(counts))
}

func (s *Server) organize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target string `json:"target"`
	}
	_ = decodeBody(w, r, &req)
	u, _ := userFromContext(r.Context())
	table := map[string]string{"wrong": "practice_records", "favorites": "favorites", "notes": "notes"}[req.Target]
	if table == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "target 不合法")
		return
	}
	_, err := s.db.ExecContext(r.Context(), `
		UPDATE `+table+` SET group_key = CASE
			WHEN subject = '公务员·行测' THEN '行测'
			WHEN subject = '事业编·职测' THEN '职测'
			WHEN subject = '公务员·申论' THEN '申论'
			WHEN subject = '事业编·综应' THEN '综应'
			WHEN subject = '自定义' THEN 'custom'
			ELSE group_key END
		WHERE user_id = ? AND group_key = ''`, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "整理失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

func currentUserID(r *http.Request) int64 {
	u, _ := userFromContext(r.Context())
	if u == nil {
		return 0
	}
	return u.ID
}

func stringifyID(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func numericID(s string) any {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return s
}

func parseSelected(raw json.RawMessage) []int {
	if len(raw) == 0 || string(raw) == "null" {
		return []int{}
	}
	var arr []int
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	var one int
	if json.Unmarshal(raw, &one) == nil {
		return []int{one}
	}
	return []int{}
}

func gradeQuestion(q *questionJSON, selected []int) checkResult {
	res := checkResult{Selected: selected, Correct: []int{}, CorrectText: []string{}}
	ans := strings.TrimSpace(q.Answer)
	if strings.HasPrefix(ans, "[") {
		var list []int
		if err := json.Unmarshal([]byte(ans), &list); err == nil {
			res.Correct = list
			ok := sameInts(list, selected)
			res.OK = &ok
			res.CorrectText = optionTexts(q.Options, list)
			return res
		}
	}
	if strings.Contains(ans, ",") && isNumberList(ans) {
		var list []int
		for _, p := range strings.Split(ans, ",") {
			n, _ := strconv.Atoi(strings.TrimSpace(p))
			list = append(list, n)
		}
		res.Correct = list
		ok := sameInts(list, selected)
		res.OK = &ok
		res.CorrectText = optionTexts(q.Options, list)
		return res
	}
	if len(q.Options) == 0 {
		if ans == "" {
			return res
		}
		n, _ := strconv.Atoi(ans)
		res.Correct = []int{n}
		res.CorrectText = []string{"正确"}
		if len(selected) == 0 || selected[0] != n {
			res.CorrectText = []string{"错误"}
		}
		ok := len(selected) > 0 && selected[0] == n
		res.OK = &ok
		return res
	}
	if q.AnswerIndex >= 0 {
		res.Correct = []int{int(q.AnswerIndex)}
		ok := len(selected) > 0 && selected[0] == int(q.AnswerIndex)
		res.OK = &ok
		res.CorrectText = optionTexts(q.Options, res.Correct)
		return res
	}
	if isNumberList(ans) {
		n, _ := strconv.Atoi(ans)
		res.Correct = []int{n}
		ok := len(selected) > 0 && selected[0] == n
		res.OK = &ok
		res.CorrectText = optionTexts(q.Options, res.Correct)
	}
	return res
}

func sameInts(want, got []int) bool {
	if len(want) != len(got) {
		return false
	}
	m := map[int]bool{}
	for _, x := range want {
		m[x] = true
	}
	for _, x := range got {
		if !m[x] {
			return false
		}
	}
	return true
}

func optionTexts(opts []string, indexes []int) []string {
	out := make([]string, 0, len(indexes))
	for _, i := range indexes {
		if i >= 0 && i < len(opts) {
			out = append(out, opts[i])
		}
	}
	return out
}

func isNumberList(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != ',' && r != ' ' {
			return false
		}
	}
	return true
}

func (s *Server) findQuestion(r *http.Request, id string) (*questionJSON, error) {
	if strings.HasPrefix(id, "custom-") {
		return nil, fmt.Errorf("custom question not migrated")
	}
	qid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(r.Context(), `
		SELECT questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions WHERE questionId = ? LIMIT 1`, qid)
	q, err := scanQuestion(row)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func subjectOfPaper(r *http.Request, s *Server, paperID int64) string {
	var subject string
	_ = s.db.QueryRowContext(r.Context(), `SELECT subjectName FROM papers WHERE id = ?`, paperID).Scan(&subject)
	return subject
}

func groupForSubject(subject string) string {
	switch subject {
	case "公务员·行测", "行测":
		return "行测"
	case "事业编·职测", "职测":
		return "职测"
	case "公务员·申论", "申论":
		return "申论"
	case "事业编·综应", "综应":
		return "综应"
	case "自定义", "custom":
		return "custom"
	default:
		return ""
	}
}

func countRows(r *http.Request, s *Server, query string, args ...any) int64 {
	var n int64
	_ = s.db.QueryRowContext(r.Context(), query, args...).Scan(&n)
	return n
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func sourceGroups(counts map[string]int64) []map[string]any {
	tabs := []struct{ key, name string }{
		{"行测", "行测"}, {"职测", "职测"}, {"申论", "申论"}, {"综应", "综应"}, {"custom", "自定义题库"},
	}
	out := make([]map[string]any, 0, len(tabs))
	for _, tab := range tabs {
		n := counts[tab.key]
		subs := []map[string]any{}
		if n > 0 {
			subs = append(subs, map[string]any{"key": "", "name": "全部", "count": n})
		}
		out = append(out, map[string]any{"key": tab.key, "name": tab.name, "count": n, "subs": subs})
	}
	return out
}
