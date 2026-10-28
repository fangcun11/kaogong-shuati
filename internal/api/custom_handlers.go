package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type customQuestionInput struct {
	Prompt      string          `json:"prompt"`
	Material    string          `json:"material"`
	Options     []string        `json:"options"`
	Answer      string          `json:"answer"`
	AnswerIndex *int64          `json:"answer_index"`
	AnswerCamel *int64          `json:"answerIndex"`
	Analysis    string          `json:"analysis"`
	Images      json.RawMessage `json:"images"`
	MaterialID  string          `json:"material_id"`
}

type customQuestionRow struct {
	ID          int64
	BatchID     int64
	Prompt      string
	Material    string
	Options     string
	Answer      string
	AnswerIndex int64
	Analysis    string
	Images      string
	MaterialID  string
}

func (s *Server) customImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string                `json:"name"`
		Subject   string                `json:"subject"`
		Questions []customQuestionInput `json:"questions"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	if strings.TrimSpace(req.Name) == "" || len(req.Questions) == 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少批次名或题目")
		return
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		subject = "自定义"
	}
	u, _ := userFromContext(r.Context())
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建批次失败")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(),
		`INSERT INTO custom_batches (user_id, name, subject) VALUES (?, ?, ?)`, u.ID, strings.TrimSpace(req.Name), subject)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建批次失败")
		return
	}
	batchID, _ := res.LastInsertId()
	stmt, err := tx.PrepareContext(r.Context(), `
		INSERT INTO custom_questions
		(batch_id, prompt, material, options, answer, answer_index, analysis, images, material_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建题目失败")
		return
	}
	defer stmt.Close()
	for _, q := range req.Questions {
		options, _ := json.Marshal(q.Options)
		if q.Options == nil {
			options = []byte("[]")
		}
		images := q.Images
		if len(images) == 0 || !json.Valid(images) {
			images = json.RawMessage("[]")
		}
		answerIndex := int64(-1)
		if q.AnswerIndex != nil {
			answerIndex = *q.AnswerIndex
		} else if q.AnswerCamel != nil {
			answerIndex = *q.AnswerCamel
		}
		if _, err := stmt.ExecContext(r.Context(), batchID, strings.TrimSpace(q.Prompt), q.Material,
			string(options), q.Answer, answerIndex, q.Analysis, string(images), q.MaterialID); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "写入题目失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "导入失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": batchID, "name": strings.TrimSpace(req.Name), "subject": subject, "count": len(req.Questions)})
}

func (s *Server) customQuestions(w http.ResponseWriter, r *http.Request) {
	batchID := queryInt64(r, "batch_id", 0)
	if batchID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 batch_id")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsBatch(r, u.ID, batchID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, batch_id, prompt, material, options, answer, answer_index, analysis, images, material_id
		FROM custom_questions WHERE batch_id = ? ORDER BY id`, batchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	defer rows.Close()
	questions := make([]map[string]any, 0)
	for rows.Next() {
		q, err := scanCustomQuestion(rows)
		if err != nil {
			continue
		}
		questions = append(questions, customQuestionJSON(q))
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": questions})
}

func (s *Server) customBatch(w http.ResponseWriter, r *http.Request) {
	id := queryInt64(r, "id", 0)
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 id")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsBatch(r, u.ID, id) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
		return
	}
	if r.Method == http.MethodDelete {
		_, err := s.db.ExecContext(r.Context(), `DELETE FROM custom_batches WHERE id = ? AND user_id = ?`, id, u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "删除批次失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var req struct {
		Name    string `json:"name"`
		Subject string `json:"subject"`
	}
	if err := decodeBody(w, r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少批次名")
		return
	}
	var err error
	if req.Subject != "" {
		_, err = s.db.ExecContext(r.Context(), `UPDATE custom_batches SET name = ?, subject = ?, updated_at = datetime('now','localtime') WHERE id = ? AND user_id = ?`, strings.TrimSpace(req.Name), req.Subject, id, u.ID)
	} else {
		_, err = s.db.ExecContext(r.Context(), `UPDATE custom_batches SET name = ?, updated_at = datetime('now','localtime') WHERE id = ? AND user_id = ?`, strings.TrimSpace(req.Name), id, u.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "修改批次失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) customBatchMerge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs  []int64 `json:"ids"`
		Name string  `json:"name"`
	}
	if err := decodeBody(w, r, &req); err != nil || len(req.IDs) < 2 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "至少选择两个批次")
		return
	}
	u, _ := userFromContext(r.Context())
	ids := uniquePositive(req.IDs)
	if len(ids) < 2 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "至少选择两个批次")
		return
	}
	for _, id := range ids {
		if !s.ownsBatch(r, u.ID, id) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
			return
		}
	}
	target := ids[0]
	tx, _ := s.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	for _, id := range ids[1:] {
		_, _ = tx.ExecContext(r.Context(), `UPDATE custom_questions SET batch_id = ? WHERE batch_id = ?`, target, id)
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM custom_batches WHERE id = ?`, id)
	}
	if strings.TrimSpace(req.Name) != "" {
		_, _ = tx.ExecContext(r.Context(), `UPDATE custom_batches SET name = ?, updated_at = datetime('now','localtime') WHERE id = ?`, strings.TrimSpace(req.Name), target)
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "合并失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": target, "count": s.batchCount(r, target)})
}

func (s *Server) customBatchSplit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BatchID     int64   `json:"batch_id"`
		QuestionIDs []int64 `json:"question_ids"`
		Name        string  `json:"name"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.BatchID <= 0 || len(req.QuestionIDs) == 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少批次或题目")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsBatch(r, u.ID, req.BatchID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
		return
	}
	var name, subject string
	if err := s.db.QueryRowContext(r.Context(), `SELECT name, subject FROM custom_batches WHERE id = ?`, req.BatchID).Scan(&name, &subject); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取批次失败")
		return
	}
	newName := strings.TrimSpace(req.Name)
	if newName == "" {
		newName = name + "-拆分"
	}
	tx, _ := s.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO custom_batches (user_id, name, subject) VALUES (?, ?, ?)`, u.ID, newName, subject)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "创建批次失败")
		return
	}
	newID, _ := res.LastInsertId()
	for _, qid := range req.QuestionIDs {
		_, _ = tx.ExecContext(r.Context(), `UPDATE custom_questions SET batch_id = ? WHERE id = ? AND batch_id = ?`, newID, qid, req.BatchID)
	}
	_ = tx.Commit()
	writeJSON(w, http.StatusOK, map[string]any{"id": newID, "name": newName, "count": s.batchCount(r, newID)})
}

func (s *Server) customQuestion(w http.ResponseWriter, r *http.Request) {
	qid := queryInt64(r, "id", 0)
	if qid <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 id")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsQuestion(r, u.ID, qid) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
		return
	}
	if r.Method == http.MethodDelete {
		_, err := s.db.ExecContext(r.Context(), `DELETE FROM custom_questions WHERE id = ?`, qid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "删除失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var req customQuestionInput
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	options, _ := json.Marshal(req.Options)
	images := req.Images
	if len(images) == 0 || !json.Valid(images) {
		images = json.RawMessage("[]")
		var existing string
		if s.db.QueryRowContext(r.Context(), `SELECT images FROM custom_questions WHERE id = ?`, qid).Scan(&existing) == nil && json.Valid([]byte(existing)) {
			images = json.RawMessage(existing)
		}
	}
	answerIndex := int64(-1)
	if req.AnswerIndex != nil {
		answerIndex = *req.AnswerIndex
	} else if req.AnswerCamel != nil {
		answerIndex = *req.AnswerCamel
	}
	_, err := s.db.ExecContext(r.Context(), `
		UPDATE custom_questions SET prompt = ?, material = ?, options = ?, answer = ?, answer_index = ?, analysis = ?, images = ?
		WHERE id = ?`, strings.TrimSpace(req.Prompt), req.Material, string(options), req.Answer, answerIndex, req.Analysis, string(images), qid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "修改失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) customGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs     []int64 `json:"ids"`
		Action  string  `json:"action"`
		GroupID string  `json:"groupId"`
	}
	if err := decodeBody(w, r, &req); err != nil || len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请先勾选题目")
		return
	}
	u, _ := userFromContext(r.Context())
	for _, id := range req.IDs {
		if !s.ownsQuestion(r, u.ID, id) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "部分题目不存在")
			return
		}
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(req.IDs)), ",")
	args := make([]any, 0, len(req.IDs))
	for _, id := range req.IDs {
		args = append(args, id)
	}
	if req.Action == "ungroup" {
		_, err := s.db.ExecContext(r.Context(), `UPDATE custom_questions SET material_id = '' WHERE id IN (`+marks+`)`, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "取消分组失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "ungroup", "count": len(req.IDs)})
		return
	}
	gid := strings.TrimSpace(req.GroupID)
	if gid == "" {
		gid = newPaperID()
	}
	args = append([]any{gid}, args...)
	_, err := s.db.ExecContext(r.Context(), `UPDATE custom_questions SET material_id = ? WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "分组失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "group", "groupId": gid, "count": len(req.IDs)})
}

func (s *Server) customAutoGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BatchID int64 `json:"batch_id"`
	}
	if err := decodeBody(w, r, &req); err != nil || req.BatchID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 batch_id")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsBatch(r, u.ID, req.BatchID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, material FROM custom_questions WHERE batch_id = ? ORDER BY id`, req.BatchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	groups := map[string][]int64{}
	total := 0
	for rows.Next() {
		var id int64
		var material string
		if rows.Scan(&id, &material) != nil {
			continue
		}
		total++
		key := normalizeMaterial(material)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], id)
	}
	rows.Close()
	grouped, groupCount := 0, 0
	tx, _ := s.db.BeginTx(r.Context(), nil)
	defer tx.Rollback()
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		gid := newPaperID()
		marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		args := []any{gid}
		for _, id := range ids {
			args = append(args, id)
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE custom_questions SET material_id = ? WHERE id IN (`+marks+`)`, args...); err == nil {
			grouped += len(ids)
			groupCount++
		}
	}
	_ = tx.Commit()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "grouped": grouped, "groupCount": groupCount, "total": total})
}

var htmlTagRE = regexp.MustCompile(`<[^>]+>`)

func normalizeMaterial(s string) string {
	s = htmlTagRE.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func (s *Server) customPractice(w http.ResponseWriter, r *http.Request) {
	batchID := queryInt64(r, "batch_id", 0)
	if batchID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 batch_id")
		return
	}
	u, _ := userFromContext(r.Context())
	if !s.ownsBatch(r, u.ID, batchID) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "批次不存在")
		return
	}
	var batchName, subject string
	_ = s.db.QueryRowContext(r.Context(), `SELECT name, subject FROM custom_batches WHERE id = ?`, batchID).Scan(&batchName, &subject)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, batch_id, prompt, material, options, answer, answer_index, analysis, images, material_id
		FROM custom_questions WHERE batch_id = ? ORDER BY id`, batchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	defer rows.Close()
	qs := make([]customQuestionRow, 0)
	for rows.Next() {
		q, err := scanCustomQuestion(rows)
		if err == nil {
			qs = append(qs, q)
		}
	}
	out := groupCustomQuestions(qs, batchName, subject)
	count := queryInt(r, "count", 0, 0, 100)
	if count > 0 && len(out) > count {
		out = trimCustomPractice(out, count)
	}
	writeJSON(w, http.StatusOK, map[string]any{"questions": out, "batch": map[string]any{"id": batchID, "name": batchName, "subject": subject}})
}

func (s *Server) customCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuestionID string          `json:"questionId"`
		Selected   json.RawMessage `json:"selected"`
		BatchID    int64           `json:"batchId"`
		Chapter    string          `json:"chapter"`
	}
	if err := decodeBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "请求格式不正确")
		return
	}
	q, err := s.findCustomQuestion(r, req.QuestionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
		return
	}
	selected := parseSelected(req.Selected)
	res := gradeCustom(q, selected)
	u, _ := userFromContext(r.Context())
	batchID := req.BatchID
	if batchID == 0 {
		batchID = q.BatchID
	}
	var batchName, batchSubject string
	if batchID > 0 && s.ownsBatch(r, u.ID, batchID) {
		_ = s.db.QueryRowContext(r.Context(), `SELECT name, subject FROM custom_batches WHERE id = ?`, batchID).Scan(&batchName, &batchSubject)
	}
	chapter := req.Chapter
	if chapter == "" {
		chapter = batchName
	}
	if batchSubject == "" {
		batchSubject = "自定义"
	}
	_, _ = s.db.ExecContext(r.Context(), `
		INSERT INTO practice_records
		(user_id, question_id, subject, chapter, question_type, selected, is_correct, cost_ms, group_key)
		VALUES (?, ?, ?, ?, 0, ?, ?, 0, ?)`,
		u.ID, req.QuestionID, batchSubject, chapter, mustJSON(selected), boolToInt(res.OK), "custom")
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) findCustomQuestion(r *http.Request, id string) (*customQuestionRow, error) {
	cid, err := strconv.ParseInt(strings.TrimPrefix(id, "custom-"), 10, 64)
	if err != nil || cid <= 0 {
		return nil, sql.ErrNoRows
	}
	u, _ := userFromContext(r.Context())
	if u == nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(r.Context(), `
		SELECT q.id, q.batch_id, q.prompt, q.material, q.options, q.answer, q.answer_index, q.analysis, q.images, q.material_id
		FROM custom_questions q
		JOIN custom_batches b ON b.id = q.batch_id
		WHERE q.id = ? AND b.user_id = ?`, cid, u.ID)
	q, err := scanCustomQuestionRow(row)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func scanCustomQuestion(rows *sql.Rows) (customQuestionRow, error) {
	return scanCustomQuestionRow(rows)
}

type customScanner interface{ Scan(...any) error }

func scanCustomQuestionRow(row customScanner) (customQuestionRow, error) {
	var q customQuestionRow
	err := row.Scan(&q.ID, &q.BatchID, &q.Prompt, &q.Material, &q.Options, &q.Answer, &q.AnswerIndex, &q.Analysis, &q.Images, &q.MaterialID)
	return q, err
}

func trimCustomPractice(questions []map[string]any, count int) []map[string]any {
	if count <= 0 || len(questions) <= count {
		return questions
	}
	end := count
	last := questions[count-1]
	gid, _ := last["groupId"].(string)
	total, _ := last["groupTotal"].(int)
	if gid != "" && total > 0 {
		for i := count; i < len(questions); i++ {
			if qgid, _ := questions[i]["groupId"].(string); qgid == gid {
				if idx, _ := questions[i]["groupIndex"].(int); idx == total-1 {
					end = i + 1
					break
				}
			}
		}
	}
	return questions[:end]
}

func customQuestionJSON(q customQuestionRow) map[string]any {
	var options []string
	_ = json.Unmarshal([]byte(q.Options), &options)
	var images any
	if json.Valid([]byte(q.Images)) {
		_ = json.Unmarshal([]byte(q.Images), &images)
	}
	if images == nil {
		images = []any{}
	}
	return map[string]any{
		"id": q.ID, "batch_id": q.BatchID, "prompt": q.Prompt, "material": q.Material,
		"options": options, "answer": q.Answer, "answer_index": q.AnswerIndex,
		"analysis": q.Analysis, "images": images, "material_id": q.MaterialID,
	}
}

type customImage struct {
	Role    string `json:"role"`
	DataURL string `json:"dataUrl"`
	URL     string `json:"url"`
}

func parseCustomImages(raw string) []customImage {
	var images []customImage
	if json.Unmarshal([]byte(raw), &images) == nil {
		return images
	}
	var urls []string
	if json.Unmarshal([]byte(raw), &urls) == nil {
		out := make([]customImage, 0, len(urls))
		for _, u := range urls {
			out = append(out, customImage{DataURL: u})
		}
		return out
	}
	return nil
}

func customImagesHTML(images []customImage, role string) string {
	var b strings.Builder
	for _, im := range images {
		if role != "" && im.Role != "" && im.Role != role {
			continue
		}
		src := strings.TrimSpace(im.DataURL)
		if src == "" {
			src = strings.TrimSpace(im.URL)
		}
		if !(strings.HasPrefix(src, "data:image/") || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")) {
			continue
		}
		b.WriteString(`<p><img src="`)
		b.WriteString(htmlEscape(src))
		b.WriteString(`" alt="题目图片"></p>`)
	}
	return b.String()
}

func customHTML(text string, images []customImage, role string) string {
	body := htmlEscape(text)
	body = strings.ReplaceAll(body, "\n", "<br>")
	img := customImagesHTML(images, role)
	if body == "" && img == "" {
		return ""
	}
	return "<p>" + body + "</p>" + img
}

func groupCustomQuestions(rows []customQuestionRow, batchName, subject string) []map[string]any {
	groups := map[string][]customQuestionRow{}
	order := make([]string, 0)
	for _, q := range rows {
		key := q.MaterialID
		if key == "" {
			key = "q:" + strconv.FormatInt(q.ID, 10)
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], q)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, key := range order {
		members := groups[key]
		holder := members[0]
		for _, m := range members {
			if strings.TrimSpace(m.Material) != "" {
				holder = m
				break
			}
		}
		holderImages := parseCustomImages(holder.Images)
		for i, q := range members {
			item := customQuestionJSON(q)
			images := parseCustomImages(q.Images)
			item["id"] = "custom-" + strconv.FormatInt(q.ID, 10)
			item["questionId"] = "custom-" + strconv.FormatInt(q.ID, 10)
			item["content"] = q.Prompt
			item["contentHtml"] = customHTML(q.Prompt, images, "stem")
			item["material"] = holder.Material
			item["materialHtml"] = customHTML(holder.Material, holderImages, "material")
			item["type"] = "custom"
			item["chapter"] = batchName
			item["subjectName"] = subject
			item["batchId"] = q.BatchID
			if q.MaterialID != "" {
				item["groupId"] = q.MaterialID
				item["groupIndex"] = i
				item["groupTotal"] = len(members)
			}
			out = append(out, item)
		}
	}
	return out
}

func gradeCustom(q *customQuestionRow, selected []int) checkResult {
	var options []string
	_ = json.Unmarshal([]byte(q.Options), &options)
	qj := questionJSON{Answer: q.Answer, AnswerIndex: q.AnswerIndex, Options: options}
	return gradeQuestion(&qj, selected)
}

func (s *Server) ownsBatch(r *http.Request, userID, batchID int64) bool {
	var n int
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM custom_batches WHERE id = ? AND user_id = ?`, batchID, userID).Scan(&n)
	return n > 0
}

func (s *Server) ownsQuestion(r *http.Request, userID, questionID int64) bool {
	var n int
	_ = s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM custom_questions q JOIN custom_batches b ON b.id = q.batch_id
		WHERE q.id = ? AND b.user_id = ?`, questionID, userID).Scan(&n)
	return n > 0
}

func (s *Server) batchCount(r *http.Request, batchID int64) int64 {
	var n int64
	_ = s.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM custom_questions WHERE batch_id = ?`, batchID).Scan(&n)
	return n
}

func uniquePositive(in []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v > 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func boolToInt(v *bool) any {
	if v == nil {
		return nil
	}
	if *v {
		return 1
	}
	return 0
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}
