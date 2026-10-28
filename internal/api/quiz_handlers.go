package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type categoryRow struct {
	Category  string `json:"category"`
	Papers    int64  `json:"papers"`
	Questions int64  `json:"questions"`
}

func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 subject")
		return
	}
	from, to := yearRange()
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT p.category, COUNT(DISTINCT p.id), COUNT(DISTINCT q.questionId)
		FROM papers p LEFT JOIN questions q ON q.paperId = p.id
		WHERE p.subjectName = ? AND substr(p.name,1,4) BETWEEN ? AND ?
		GROUP BY p.category ORDER BY COUNT(DISTINCT p.id) DESC, p.category`, subject, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取分类失败")
		return
	}
	defer rows.Close()
	out := make([]categoryRow, 0)
	for rows.Next() {
		var x categoryRow
		if err := rows.Scan(&x.Category, &x.Papers, &x.Questions); err == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type paperRow struct {
	ID            int64   `json:"id"`
	Category      string  `json:"category"`
	Name          string  `json:"name"`
	QuestionCount int64   `json:"questionCount"`
	Difficulty    float64 `json:"difficulty"`
}

func (s *Server) papers(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 subject")
		return
	}
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	limit := queryInt(r, "limit", 50, 1, 500)
	from, to := yearRange()
	query := `SELECT id, category, name, COALESCE(questionCount,0), COALESCE(difficulty,0)
		FROM papers WHERE subjectName = ? AND substr(name,1,4) BETWEEN ? AND ?`
	args := []any{subject, from, to}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取试卷失败")
		return
	}
	defer rows.Close()
	out := make([]paperRow, 0)
	for rows.Next() {
		var x paperRow
		if err := rows.Scan(&x.ID, &x.Category, &x.Name, &x.QuestionCount, &x.Difficulty); err == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) paperByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_ID", "试卷 id 不合法")
		return
	}
	var p struct {
		ID            int64           `json:"id"`
		SubjectName   string          `json:"subjectName"`
		Category      string          `json:"category"`
		Name          string          `json:"name"`
		QuestionCount int64           `json:"questionCount"`
		Difficulty    float64         `json:"difficulty"`
		Chapters      json.RawMessage `json:"chapters"`
	}
	var chapters sql.NullString
	err = s.db.QueryRowContext(r.Context(), `
		SELECT id, subjectName, category, name, COALESCE(questionCount,0), COALESCE(difficulty,0), chapters
		FROM papers WHERE id = ?`, id).
		Scan(&p.ID, &p.SubjectName, &p.Category, &p.Name, &p.QuestionCount, &p.Difficulty, &chapters)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "试卷不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取试卷失败")
		return
	}
	if chapters.Valid && json.Valid([]byte(chapters.String)) {
		p.Chapters = json.RawMessage(chapters.String)
	} else {
		p.Chapters = json.RawMessage("[]")
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions WHERE paperId = ? ORDER BY id`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	defer rows.Close()
	questions := make([]questionJSON, 0)
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
			return
		}
		questions = append(questions, q)
	}
	questions = s.enrichMaterials(r, p.SubjectName, questions)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": p.ID, "subjectName": p.SubjectName, "category": p.Category, "name": p.Name,
		"questionCount": p.QuestionCount, "difficulty": p.Difficulty, "chapters": p.Chapters,
		"questions": questions,
	})
}

type materialBlock struct {
	Title string `json:"title"`
	Idx   int64  `json:"idx"`
	Text  string `json:"text"`
}

func (s *Server) materials(w http.ResponseWriter, r *http.Request) {
	paperID := queryInt64(r, "paperId", 0)
	if paperID <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 paperId")
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT title, idx, text FROM materials WHERE paperId = ? ORDER BY idx`, paperID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取材料失败")
		return
	}
	defer rows.Close()
	out := make([]materialBlock, 0)
	for rows.Next() {
		var x materialBlock
		if err := rows.Scan(&x.Title, &x.Idx, &x.Text); err == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type chapterGroup struct {
	Group    string        `json:"group"`
	Total    int64         `json:"total"`
	Done     int64         `json:"done"`
	Rate     any           `json:"rate"`
	Chapters []string      `json:"chapters"`
	Subs     []chapterNode `json:"subs"`
}

type chapterNode struct {
	Name     string   `json:"name"`
	Total    int64    `json:"total"`
	Done     int64    `json:"done"`
	Rate     any      `json:"rate"`
	Chapters []string `json:"chapters"`
	Leaves   []any    `json:"leaves"`
}

func (s *Server) chaptersLegacy(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 subject")
		return
	}
	u, _ := userFromContext(r.Context())
	from, to := yearRange()
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT q.chapter, COUNT(DISTINCT q.questionId) c
		FROM questions q JOIN papers p ON p.id = q.paperId
		WHERE p.subjectName = ? AND substr(p.name,1,4) BETWEEN ? AND ? AND q.chapter <> ''
		GROUP BY q.chapter ORDER BY c DESC`, subject, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取章节失败")
		return
	}
	defer rows.Close()
	type chapterCount struct {
		name  string
		total int64
	}
	counts := make([]chapterCount, 0)
	for rows.Next() {
		var x chapterCount
		if err := rows.Scan(&x.name, &x.total); err == nil {
			counts = append(counts, x)
		}
	}
	done := map[string]int64{}
	drows, err := s.db.QueryContext(r.Context(), `
		SELECT chapter, COUNT(*) c FROM practice_records
		WHERE user_id = ? AND is_correct IS NOT NULL GROUP BY chapter`, u.ID)
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

	order := make([]string, 0)
	byGroup := map[string][]chapterCount{}
	for _, c := range counts {
		g := groupForChapter(subject, c.name)
		if _, ok := byGroup[g]; !ok {
			order = append(order, g)
		}
		byGroup[g] = append(byGroup[g], c)
	}
	groups := make([]chapterGroup, 0, len(order))
	for _, g := range order {
		chapters := make([]string, 0, len(byGroup[g]))
		var total, doneN int64
		for _, c := range byGroup[g] {
			chapters = append(chapters, c.name)
			total += c.total
			doneN += done[c.name]
		}
		groups = append(groups, chapterGroup{
			Group: g, Total: total, Done: doneN, Rate: nil, Chapters: []string{},
			Subs: []chapterNode{{
				Name: "全部", Total: total, Done: doneN, Rate: nil,
				Chapters: chapters, Leaves: []any{},
			}},
		})
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) chapters(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 subject")
		return
	}
	u, _ := userFromContext(r.Context())
	from, to := yearRange()
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT qc.category, qc.sub, COUNT(DISTINCT qc.question_id) c
		FROM question_categories qc
		JOIN questions q ON q.questionId = qc.question_id
		JOIN papers p ON p.id = q.paperId
		WHERE qc.subject = ? AND substr(p.name,1,4) BETWEEN ? AND ?
		GROUP BY qc.category, qc.sub ORDER BY c DESC`, subject, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取章节失败")
		return
	}
	defer rows.Close()
	type catCount struct {
		group, sub string
		total      int64
	}
	counts := make([]catCount, 0)
	for rows.Next() {
		var x catCount
		if err := rows.Scan(&x.group, &x.sub, &x.total); err == nil {
			counts = append(counts, x)
		}
	}
	if len(counts) == 0 {
		s.chaptersLegacy(w, r)
		return
	}
	done := map[string]int64{}
	drows, err := s.db.QueryContext(r.Context(), `
		SELECT qc.category, qc.sub, COUNT(*) c
		FROM practice_records r
		JOIN question_categories qc ON qc.question_id = CAST(r.question_id AS INTEGER) AND qc.subject = ?
		WHERE r.user_id = ? AND r.is_correct IS NOT NULL
		GROUP BY qc.category, qc.sub`, subject, u.ID)
	if err == nil {
		defer drows.Close()
		for drows.Next() {
			var group, sub string
			var n int64
			if err := drows.Scan(&group, &sub, &n); err == nil {
				done[group+"|"+sub] = n
			}
		}
	}
	order := groupNames(subject)
	seen := map[string]bool{}
	for _, n := range order {
		seen[n] = true
	}
	for _, c := range counts {
		if !seen[c.group] {
			seen[c.group] = true
			order = append(order, c.group)
		}
	}
	groups := make([]chapterGroup, 0, len(order))
	for _, g := range order {
		subs := make([]chapterNode, 0)
		var total, doneN int64
		for _, c := range counts {
			if c.group != g {
				continue
			}
			total += c.total
			doneN += done[g+"|"+c.sub]
			subs = append(subs, chapterNode{
				Name: c.sub, Total: c.total, Done: done[g+"|"+c.sub], Rate: nil,
				Chapters: []string{}, Leaves: []any{},
			})
		}
		if len(subs) == 0 {
			continue
		}
		groups = append(groups, chapterGroup{
			Group: g, Total: total, Done: doneN, Rate: nil, Chapters: []string{}, Subs: subs,
		})
	}
	writeJSON(w, http.StatusOK, groups)
}

func groupNames(subject string) []string {
	switch subject {
	case "公务员·行测", "事业编·职测":
		return []string{"政治理论", "常识判断", "言语理解与表达", "数量关系", "判断推理", "资料分析"}
	case "公务员·申论":
		return []string{"归纳概括题", "综合分析题", "提出对策题", "贯彻执行题", "文章写作题"}
	case "事业编·综应":
		return []string{"A类·综合管理"}
	default:
		return nil
	}
}

func groupForChapter(subject, chapter string) string {
	type groupSpec struct {
		name string
		keys []string
	}
	var specs []groupSpec
	switch subject {
	case "公务员·行测", "事业编·职测":
		specs = []groupSpec{
			{"政治理论", []string{"政治理论"}},
			{"常识判断", []string{"常识判断", "常识", "公共基础知识", "综合知识", "科学推理", "科学素养", "法律基础知识", "综合分析能力"}},
			{"言语理解与表达", []string{"言语理解与表达", "言语理解", "言语表达与理解", "言语应用", "选词填空", "阅读理解", "语句表达", "段落阅读"}},
			{"数量关系", []string{"数量关系", "数学运算", "数字推理", "数理能力", "数量分析"}},
			{"判断推理", []string{"判断推理", "逻辑判断", "图形推理", "定义判断", "类比推理", "逻辑推理", "演绎推理"}},
			{"资料分析", []string{"资料分析", "资料分斩"}},
		}
	case "公务员·申论":
		specs = []groupSpec{
			{"归纳概括题", []string{"归纳概括"}},
			{"综合分析题", []string{"综合分析"}},
			{"提出对策题", []string{"提出对策"}},
			{"贯彻执行题", []string{"贯彻执行"}},
			{"文章写作题", []string{"文章写作", "大作文"}},
		}
	case "事业编·综应":
		specs = []groupSpec{{"A类·综合管理", []string{"A类", "综合管理", "归纳概括", "提出对策", "综合分析", "案例分析", "应急处理", "沟通协调", "应用文"}}}
	}
	for _, g := range specs {
		for _, k := range g.keys {
			if chapter == k || strings.Contains(chapter, k) {
				return g.name
			}
		}
	}
	return chapter
}

type questionJSON struct {
	ID           int64    `json:"id"`
	QuestionID   int64    `json:"questionId"`
	PaperID      int64    `json:"paperId"`
	Chapter      string   `json:"chapter"`
	Type         int64    `json:"type"`
	Content      string   `json:"content"`
	ContentHTML  string   `json:"contentHtml"`
	Options      []string `json:"options"`
	Answer       string   `json:"answer"`
	AnswerIndex  int64    `json:"answerIndex"`
	Difficulty   int64    `json:"difficulty"`
	Analysis     string   `json:"analysis"`
	Material     string   `json:"material,omitempty"`
	MaterialHTML string   `json:"materialHtml,omitempty"`
	Module       string   `json:"module,omitempty"`
	GroupID      string   `json:"groupId,omitempty"`
	GroupIndex   int      `json:"groupIndex"`
	GroupTotal   int      `json:"groupTotal,omitempty"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanQuestion(row rowScanner) (questionJSON, error) {
	var q questionJSON
	var chapter, contentHTML, options, answer, analysis sql.NullString
	var qtype, answerIndex, difficulty sql.NullInt64
	if err := row.Scan(&q.QuestionID, &q.PaperID, &chapter, &qtype, &q.Content, &contentHTML, &options, &answer, &answerIndex, &difficulty, &analysis); err != nil {
		return q, err
	}
	q.ID = q.QuestionID
	q.Chapter = chapter.String
	q.Type = qtype.Int64
	q.ContentHTML = contentHTML.String
	q.Answer = answer.String
	q.AnswerIndex = answerIndex.Int64
	q.Difficulty = difficulty.Int64
	q.Analysis = analysis.String
	q.Options = parseOptions(options.String)
	return q, nil
}

func parseOptions(raw string) []string {
	var out []string
	if raw == "" {
		return []string{}
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func (s *Server) question(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("id")
	if strings.HasPrefix(raw, "custom-") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "自定义题接口尚未迁移")
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "BAD_ID", "题目 id 不合法")
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		SELECT questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions WHERE questionId = ? LIMIT 1`, id)
	q, err := scanQuestion(row)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "题目不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	var subject string
	_ = s.db.QueryRowContext(r.Context(), `SELECT subjectName FROM papers WHERE id = ?`, q.PaperID).Scan(&subject)
	enriched := s.enrichMaterials(r, subject, []questionJSON{q})
	writeJSON(w, http.StatusOK, enriched[0])
}

func (s *Server) practice(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject"))
	if subject == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 subject")
		return
	}
	n := queryInt(r, "n", 10, 1, 50)
	custom := r.URL.Query().Get("custom") == "1"
	chapters := parseCSV(r.URL.Query().Get("chapters"))
	if len(chapters) == 0 {
		if ch := strings.TrimSpace(r.URL.Query().Get("chapter")); ch != "" {
			chapters = []string{ch}
		}
	}
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	sub := strings.TrimSpace(r.URL.Query().Get("sub"))
	year := strings.TrimSpace(r.URL.Query().Get("year"))
	difficulty := strings.TrimSpace(r.URL.Query().Get("difficulty"))
	mock := strings.TrimSpace(r.URL.Query().Get("mock"))

	fetchN := n
	if custom {
		fetchN = n * 3
		if fetchN > 150 {
			fetchN = 150
		}
	}
	from, to := yearRangeFor(year)
	diffSQL, diffArgs := difficultyWhere(difficulty)
	mockSQL := mockWhere(mock)

	where := []string{"p.subjectName = ?", "substr(p.name,1,4) BETWEEN ? AND ?"}
	args := []any{subject, from, to}
	where = append(where, diffSQL, mockSQL)
	args = append(args, diffArgs...)

	var ids []int64
	var err error
	if group != "" {
		where = append(where, "qc.category = ?")
		args = append(args, group)
		if sub != "" && sub != "全部" {
			where = append(where, "qc.sub = ?")
			args = append(args, sub)
		}
		args = append(args, fetchN)
		ids, err = s.queryQuestionIDs(r, `SELECT qc.question_id FROM question_categories qc
			JOIN questions q ON q.questionId = qc.question_id
			JOIN papers p ON p.id = q.paperId
			WHERE `+strings.Join(where, " AND ")+`
			GROUP BY qc.question_id ORDER BY RANDOM() LIMIT ?`, args...)
	} else if len(chapters) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(chapters)), ",")
		where = append(where, "q.chapter IN ("+marks+")")
		for _, c := range chapters {
			args = append(args, c)
		}
		args = append(args, fetchN)
		ids, err = s.queryQuestionIDs(r, `SELECT q.questionId FROM questions q
			JOIN papers p ON p.id = q.paperId
			WHERE `+strings.Join(where, " AND ")+`
			GROUP BY q.questionId ORDER BY RANDOM() LIMIT ?`, args...)
	} else {
		args = append(args, fetchN)
		ids, err = s.queryQuestionIDs(r, `SELECT q.questionId FROM questions q
			JOIN papers p ON p.id = q.paperId
			WHERE `+strings.Join(where, " AND ")+`
			GROUP BY q.questionId ORDER BY RANDOM() LIMIT ?`, args...)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "抽题失败")
		return
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, []questionJSON{})
		return
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	queryArgs := make([]any, 0, len(ids))
	for _, id := range ids {
		queryArgs = append(queryArgs, id)
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT questionId, paperId, chapter, type, content, contentHtml, options, answer, answerIndex, difficulty, analysis
		FROM questions WHERE questionId IN (`+marks+`) GROUP BY questionId`, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "读取题目失败")
		return
	}
	defer rows.Close()
	out := make([]questionJSON, 0, len(ids))
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err == nil {
			out = append(out, q)
		}
	}
	out = s.enrichMaterials(r, subject, out)
	out = s.enrichPracticeGroups(r, subject, out)
	max := practiceMax(subject, group, chapters)
	if custom {
		max = n
	}
	out = trimPracticeQuestions(out, max)
	writeJSON(w, http.StatusOK, out)
}

func yearRangeFor(year string) (string, string) {
	to := time.Now().Year()
	if year == "all" {
		return "0000", itoa(to)
	}
	n := 10
	if year != "" {
		if v, err := strconv.Atoi(year); err == nil && v >= 3 {
			n = v
		}
	}
	return itoa(to - n + 1), itoa(to)
}

func difficultyWhere(d string) (string, []any) {
	min, max := difficultyRange(d)
	if d == "" || d == "random" {
		return "1=1", nil
	}
	return "q.difficulty BETWEEN ? AND ?", []any{min, max}
}

func mockWhere(mock string) string {
	switch mock {
	case "0":
		return "(p.category NOT LIKE '%模拟%' AND p.category NOT LIKE '%模考%')"
	case "1":
		return "(p.category LIKE '%模拟%' OR p.category LIKE '%模考%')"
	default:
		return "1=1"
	}
}

func practiceMax(subject, group string, chapters []string) int {
	if subject == "公务员·申论" || subject == "事业编·综应" {
		return 2
	}
	names := append([]string{group}, chapters...)
	for _, name := range names {
		if strings.Contains(name, "资料") || strings.Contains(name, "判断") || strings.Contains(name, "言语") {
			return 20
		}
	}
	return 15
}

func (s *Server) enrichPracticeGroups(r *http.Request, subject string, qs []questionJSON) []questionJSON {
	if len(qs) == 0 {
		return qs
	}
	ids := make([]any, 0, len(qs))
	for _, q := range qs {
		ids = append(ids, q.QuestionID)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT mm.question_id, mm.material_id FROM q_material_map mm
		JOIN q_materials m ON m.material_id = mm.material_id AND m.subject = mm.subject
		WHERE mm.subject = ? AND mm.material_id IS NOT NULL AND mm.question_id IN (`+marks+`)`, append([]any{subject}, ids...)...)
	if err != nil {
		return qs
	}
	defer rows.Close()
	groups := map[int64]string{}
	for rows.Next() {
		var qid int64
		var gid string
		if rows.Scan(&qid, &gid) == nil {
			groups[qid] = gid
		}
	}
	counts := map[string]int{}
	for _, gid := range groups {
		counts[gid]++
	}
	indexes := map[string]int{}
	for i := range qs {
		gid, ok := groups[qs[i].QuestionID]
		if !ok {
			continue
		}
		qs[i].GroupID = gid
		qs[i].GroupIndex = indexes[gid]
		qs[i].GroupTotal = counts[gid]
		indexes[gid]++
	}
	return qs
}

func trimPracticeQuestions(qs []questionJSON, max int) []questionJSON {
	if max <= 0 || len(qs) <= max {
		return qs
	}
	out := append([]questionJSON(nil), qs...)
	for i := len(out) - 1; i >= 0 && len(out) > max; i-- {
		if out[i].GroupID == "" {
			out = append(out[:i], out[i+1:]...)
		}
	}
	if len(out) > max {
		gids := map[string]bool{}
		for _, q := range out {
			if q.GroupID != "" {
				gids[q.GroupID] = true
			}
		}
		for gid := range gids {
			if len(out) <= max {
				break
			}
			next := out[:0]
			for _, q := range out {
				if q.GroupID != gid {
					next = append(next, q)
				}
			}
			out = next
		}
	}
	return out
}

func (s *Server) enrichMaterials(r *http.Request, subject string, qs []questionJSON) []questionJSON {
	if len(qs) == 0 {
		return qs
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(qs)), ",")
	args := []any{subject}
	for _, q := range qs {
		args = append(args, q.QuestionID)
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT map.question_id, mat.content
		FROM q_material_map map
		JOIN q_materials mat ON mat.material_id = map.material_id AND mat.subject = map.subject
		WHERE map.subject = ? AND map.question_id IN (`+marks+`)`, args...)
	if err != nil {
		return qs
	}
	defer rows.Close()
	byID := make(map[int64]string)
	for rows.Next() {
		var id int64
		var content sql.NullString
		if err := rows.Scan(&id, &content); err == nil && content.Valid && content.String != "" {
			byID[id] = content.String
		}
	}
	for i := range qs {
		if m := byID[qs[i].QuestionID]; m != "" {
			qs[i].Material = m
			qs[i].MaterialHTML = m
		}
	}
	return qs
}

func (s *Server) queryQuestionIDs(r *http.Request, query string, args ...any) ([]int64, error) {
	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func queryInt(r *http.Request, key string, fallback, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func queryInt64(r *http.Request, key string, fallback int64) int64 {
	v, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func parseCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
